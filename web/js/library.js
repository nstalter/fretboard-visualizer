// The account area and the saved-song library: folders, songs and their sections (saved progressions).
// Every server-provided string (names, email) goes in with textContent, setAttribute or new Option, never innerHTML.
//
// main.js supplies { snapshot, load, showError }:
//   snapshot() -> the current progression and settings in the shape the server stores
//   load(data) -> replaces the progression and settings with such data (async)

import * as api from './api.js';
import { iconButton } from './progression.js';

const $ = (id) => document.getElementById(id);

let host;
let me = { enabled: false, authenticated: false };
let lib = { folders: [], songs: [] };
let loaded = null;     // { id, songId }: the saved section the progression came from, or was last saved as
let baseline = null;   // JSON of the snapshot at that moment; null when no section is loaded
let busy = false;      // one library action at a time
let loginFailed = new URLSearchParams(location.search).get('login') === 'failed';
const open = new Map(); // expanded state: folders "f<id>" ("f0" = no folder, open by default), songs "s<id>" (closed by default)

const plural = (n, word) => `${n} ${word}${n === 1 ? '' : 's'}`;
const note = (text) => { $('save-status').textContent = text; };

function el(tag, className) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  return node;
}

export async function initLibrary(h) {
  host = h;
  $('new-folder').addEventListener('click', () => run(addFolder));
  $('new-song').addEventListener('click', () => run(() => addSong()));
  $('save-open').addEventListener('click', toggleSaveForm);
  $('save-cancel').addEventListener('click', () => { setSaveForm(false); $('save-open').focus(); });
  $('save-song').addEventListener('change', syncSaveForm);
  $('save-form').addEventListener('submit', (ev) => { ev.preventDefault(); run(saveNew); });
  $('save-update').addEventListener('click', () => run(updateLoaded));

  if (loginFailed) history.replaceState(null, '', location.pathname);
  await restoreDraft();
  if (await loadMe()) await run(refresh);
}

// ---- account ----

// loadMe reads /api/me, redraws the account area and shows the library only when signed in.
// It returns whether the user is signed in.
async function loadMe() {
  try {
    me = await api.me();
  } catch {
    me = { enabled: false, authenticated: false }; // an old or unreachable server: the app works as it always did
  }
  const signedIn = Boolean(me.enabled && me.authenticated);
  renderAccount();
  $('library').hidden = $('save-open').hidden = !signedIn;
  if (!signedIn) {
    setSaveForm(false);
    lib = { folders: [], songs: [] };
    loaded = baseline = null;
    note('');
    render();
  }
  return signedIn;
}

function renderAccount() {
  const box = $('account');
  box.replaceChildren();
  box.hidden = !me.enabled;
  if (!me.enabled) return;
  if (me.authenticated) {
    const email = el('span', 'account-email');
    email.textContent = me.email ?? '';
    const out = el('button');
    out.textContent = 'Sign out';
    out.addEventListener('click', signOut);
    box.append(email, out);
    return;
  }
  if (loginFailed) {
    const failed = el('span', 'account-error');
    failed.setAttribute('role', 'alert');
    failed.textContent = 'Sign-in failed. Please try again.';
    box.append(failed);
    loginFailed = false;
  }
  const link = el('a');
  link.href = '/auth/login';
  link.textContent = 'Sign in';
  link.addEventListener('click', stashDraft);
  const why = el('span');
  why.textContent = 'to save songs and progressions';
  box.append(link, why);
}

async function signOut() {
  if (isDirty() && !confirm('You have an unsaved progression. Sign out anyway?')) return;
  try {
    const { logoutUrl } = await api.logout();
    location.assign(logoutUrl);
  } catch (err) {
    host.showError(err);
  }
}

// Signing in leaves the page and comes back to a fresh one, so carry the progression being built across.
function stashDraft() {
  try {
    const data = host.snapshot();
    if (data.steps.length) sessionStorage.setItem('fv-draft', JSON.stringify(data));
  } catch { /* storage unavailable: nothing to carry */ }
}

async function restoreDraft() {
  let data = null;
  try {
    data = JSON.parse(sessionStorage.getItem('fv-draft'));
    sessionStorage.removeItem('fv-draft');
  } catch { /* storage unavailable or unreadable */ }
  if (!data) return;
  try {
    await host.load(data);
  } catch (err) {
    host.showError(err);
  }
}

// ---- running actions ----

// run does one library action at a time. A 401 means the session ended; any other error is shown
// and the library is re-read, since it may be stale (and a <select> must snap back).
async function run(fn) {
  if (busy) return;
  busy = true;
  host.showError(null);
  note('');
  try {
    await fn();
  } catch (err) {
    if (err.status === 401) {
      await loadMe();
      host.showError(me.authenticated ? err : new Error('You are signed out. Sign in again to use your library.'));
    } else {
      host.showError(err);
      await refresh().catch(() => {});
    }
  } finally {
    busy = false;
  }
}

async function refresh() {
  lib = await api.library();
  if (loaded && !findSection(loaded.id)) loaded = baseline = null;
  render();
}

function findSection(id) {
  for (const song of lib.songs) {
    const p = song.progressions.find((q) => q.id === id);
    if (p) return { song, p };
  }
  return null;
}

const songsIn = (folderId) => lib.songs.filter((s) => (s.folderId ?? null) === folderId);

// ---- library actions ----

// askName returns a trimmed, non-empty name, or null if the prompt was cancelled or left blank.
function askName(title, initial = '') {
  return prompt(title, initial)?.trim() || null;
}

async function addFolder() {
  const name = askName('New folder name');
  if (!name) return;
  await api.createFolder(name);
  await refresh();
}

async function renameFolder(folder) {
  const name = askName('Rename folder', folder.name);
  if (!name || name === folder.name) return;
  await api.renameFolder(folder.id, name);
  await refresh();
}

async function removeFolder(folder) {
  if (!confirm(`Delete the folder “${folder.name}”? Its songs are kept and become unfiled.`)) return;
  await api.deleteFolder(folder.id);
  await refresh();
}

async function addSong(folderId) {
  const name = askName('New song name');
  if (!name) return;
  await api.createSong({ name, folderId });
  if (folderId !== undefined) open.set(`f${folderId}`, true);
  await refresh();
}

async function renameSong(song) {
  const name = askName('Rename song', song.name);
  if (!name || name === song.name) return;
  await api.updateSong(song.id, { name });
  await refresh();
}

async function moveSong(song, value) {
  if (value !== '') open.set(`f${value}`, true);
  await api.updateSong(song.id, { folderId: value === '' ? null : Number(value) });
  await refresh();
}

async function removeSong(song) {
  const n = song.progressions.length;
  const and = n ? ` and its ${plural(n, 'section')}` : '';
  if (!confirm(`Delete the song “${song.name}”${and}? This cannot be undone.`)) return;
  await api.deleteSong(song.id);
  await refresh();
}

async function renameSection(p) {
  const name = askName('Rename section', p.name);
  if (!name || name === p.name) return;
  await api.updateProgression(p.id, { name });
  await refresh();
}

async function moveSection(song, p, delta) {
  const ids = song.progressions.map((q) => q.id);
  const i = ids.indexOf(p.id);
  [ids[i], ids[i + delta]] = [ids[i + delta], ids[i]];
  await api.reorderProgressions(song.id, ids);
  await refresh();
}

async function removeSection(p) {
  if (!confirm(`Delete the section “${p.name}”? This cannot be undone.`)) return;
  await api.deleteProgression(p.id);
  await refresh();
}

// isDirty: the progression on screen differs from the loaded section, or is unsaved work with nothing loaded.
function isDirty() {
  const now = host.snapshot();
  return baseline === null ? now.steps.length > 0 : JSON.stringify(now) !== baseline;
}

async function loadSection(p) {
  if (isDirty() && !confirm(`Replace the progression you are editing with “${p.name}”? Unsaved changes will be lost.`)) return;
  const saved = await api.getProgression(p.id);
  await host.load(saved.data);
  loaded = { id: p.id, songId: saved.songId };
  baseline = JSON.stringify(host.snapshot());
  render();
  note(`Loaded “${p.name}”.`);
}

// ---- saving ----

// payload is what gets saved: the current progression and settings. A chord with no playable shape
// is saved too (its fingering is null, as in the editor).
function payload() {
  const data = host.snapshot();
  if (!data.steps.length) throw new Error('Add at least one chord before saving.');
  return data;
}

function setSaveForm(show) {
  $('save-form').hidden = !show;
  $('save-open').setAttribute('aria-expanded', String(show));
}

function toggleSaveForm() {
  const show = $('save-form').hidden;
  setSaveForm(show);
  if (!show) return;
  note('');
  if (loaded) $('save-song').value = String(loaded.songId);
  if (!$('save-name').value) $('save-name').value = 'Verse';
  syncSaveForm();
  $('save-song').focus();
}

// syncSaveForm shows the new-song fields only while "New song…" is chosen.
function syncSaveForm() {
  const isNew = $('save-song').value === '';
  $('save-title-label').hidden = $('save-folder-label').hidden = !isNew;
  $('save-title').required = isNew;
}

async function saveNew() {
  const data = payload();
  const name = $('save-name').value.trim();
  if (!name) throw new Error('Name the section.');
  let songId = $('save-song').value;
  if (songId === '') {
    const title = $('save-title').value.trim();
    if (!title) throw new Error('Name the new song.');
    const folderId = $('save-folder').value;
    const song = await api.createSong({ name: title, folderId: folderId === '' ? undefined : Number(folderId) });
    await refresh();
    // From here the song exists: select it, so a failed save below is retried into it rather than creating another.
    $('save-song').value = songId = String(song.id);
    $('save-title').value = '';
    syncSaveForm();
  }
  const prog = await api.createProgression(songId, { name, data });
  loaded = { id: prog.id, songId: Number(songId) };
  baseline = JSON.stringify(data);
  open.set(`s${songId}`, true);
  $('save-name').value = '';
  setSaveForm(false);
  await refresh();
  note(`Saved “${name}” to “${findSection(prog.id).song.name}”.`);
  $('save-open').focus();
}

async function updateLoaded() {
  const current = loaded && findSection(loaded.id);
  if (!current) return;
  const data = payload();
  await api.updateProgression(loaded.id, { data });
  baseline = JSON.stringify(data);
  setSaveForm(false);
  await refresh();
  note(`Updated “${current.p.name}” in “${current.song.name}”.`);
  $('save-open').focus();
}

// ---- rendering ----

function render() {
  renderTree();
  renderSaveForm();
  $('library-empty').hidden = lib.songs.length > 0;
}

// renderSaveForm rebuilds the song and folder choices, keeping what is selected.
function renderSaveForm() {
  const songs = $('save-song');
  const keepSong = songs.value;
  songs.replaceChildren(new Option('New song…', ''));
  songs.append(...songsIn(null).map((s) => new Option(s.name, String(s.id))));
  for (const folder of lib.folders) {
    const inFolder = songsIn(folder.id);
    if (!inFolder.length) continue;
    const group = el('optgroup');
    group.label = folder.name;
    group.append(...inFolder.map((s) => new Option(s.name, String(s.id))));
    songs.append(group);
  }
  songs.value = keepSong;
  if (songs.selectedIndex < 0) songs.value = '';

  const folders = $('save-folder');
  const keepFolder = folders.value;
  folders.replaceChildren(new Option('No folder', ''), ...lib.folders.map((f) => new Option(f.name, String(f.id))));
  folders.value = keepFolder;
  if (folders.selectedIndex < 0) folders.value = '';

  const current = loaded && findSection(loaded.id);
  const update = $('save-update');
  update.hidden = !current;
  if (current) {
    update.textContent = `Update “${current.p.name}”`;
    update.title = `Overwrite the saved section “${current.p.name}” in “${current.song.name}” with the current progression`;
  }
  syncSaveForm();
}

// tool is a small icon button that runs fn as a library action. Its key lets focus survive a redraw.
function tool(key, text, label, fn, disabled = false) {
  const b = iconButton(text, label, disabled, () => run(fn));
  b.dataset.key = key;
  return b;
}

function toggle(key, name, count, isOpen) {
  const b = el('button', 'lib-toggle');
  b.dataset.key = key;
  b.setAttribute('aria-expanded', String(isOpen));
  const arrow = el('span');
  arrow.setAttribute('aria-hidden', 'true');
  arrow.textContent = isOpen ? '▾' : '▸';
  const label = el('span', 'lib-name');
  label.textContent = name;
  const n = el('span', 'count');
  n.textContent = count;
  b.append(arrow, label, n);
  b.addEventListener('click', () => { open.set(key, !isOpen); renderTree(); });
  return b;
}

function renderTree() {
  const tree = $('library-tree');
  const focused = tree.contains(document.activeElement) ? document.activeElement.dataset.key : null;
  const list = el('ul', 'lib-list');
  for (const folder of lib.folders) list.append(groupItem(folder, songsIn(folder.id)));
  const unfiled = songsIn(null);
  if (unfiled.length) list.append(groupItem(null, unfiled));
  tree.replaceChildren(list);
  if (focused) tree.querySelector(`[data-key="${focused}"]`)?.focus();
}

// groupItem is a folder with its songs, or (folder null) the songs in no folder.
function groupItem(folder, songs) {
  const key = `f${folder ? folder.id : 0}`;
  const isOpen = open.get(key) ?? true;
  const li = el('li');
  const row = el('div', 'lib-row');
  row.append(toggle(key, folder ? folder.name : 'No folder', plural(songs.length, 'song'), isOpen));
  if (folder) {
    const actions = el('span', 'lib-actions');
    actions.append(
      tool(`${key}:rename`, '✎', `Rename folder “${folder.name}”`, () => renameFolder(folder)),
      tool(`${key}:add`, '+', `Add a song to folder “${folder.name}”`, () => addSong(folder.id)),
      tool(`${key}:del`, '✕', `Delete folder “${folder.name}”`, () => removeFolder(folder)),
    );
    row.append(actions);
  }
  li.append(row);
  if (isOpen && songs.length) {
    const ul = el('ul', 'lib-songs');
    ul.append(...songs.map(songItem));
    li.append(ul);
  }
  return li;
}

function songItem(song) {
  const key = `s${song.id}`;
  const isOpen = open.get(key) ?? false;
  const li = el('li');
  const row = el('div', 'lib-row');
  row.append(toggle(key, song.name, plural(song.progressions.length, 'section'), isOpen));

  const folder = el('select');
  folder.dataset.key = `${key}:folder`;
  folder.setAttribute('aria-label', `Folder for “${song.name}”`);
  folder.append(new Option('No folder', ''), ...lib.folders.map((f) => new Option(f.name, String(f.id))));
  folder.value = song.folderId === null ? '' : String(song.folderId);
  folder.addEventListener('change', () => run(() => moveSong(song, folder.value)));

  const actions = el('span', 'lib-actions');
  actions.append(
    folder,
    tool(`${key}:rename`, '✎', `Rename song “${song.name}”`, () => renameSong(song)),
    tool(`${key}:del`, '✕', `Delete song “${song.name}”`, () => removeSong(song)),
  );
  row.append(actions);
  li.append(row);

  if (isOpen) {
    if (song.progressions.length) {
      const ol = el('ol', 'steps lib-sections');
      ol.append(...song.progressions.map((p, i) => sectionItem(song, p, i)));
      li.append(ol);
    } else {
      const hint = el('p', 'hint lib-hint');
      hint.textContent = 'No sections yet. Build a progression and press “Save to song…”.';
      li.append(hint);
    }
  }
  return li;
}

// sectionItem reuses the progression's step pill: the name loads the section into the progression panel.
function sectionItem(song, p, i) {
  const key = `p${p.id}`;
  const isLoaded = loaded?.id === p.id;
  const li = el('li', isLoaded ? 'step selected' : 'step');
  const name = el('button', 'step-name');
  name.dataset.key = key;
  name.textContent = p.name;
  name.title = 'Load into the progression';
  if (isLoaded) name.setAttribute('aria-current', 'true');
  name.addEventListener('click', () => run(() => loadSection(p)));
  li.append(
    name,
    tool(`${key}:up`, '↑', `Move section “${p.name}” up`, () => moveSection(song, p, -1), i === 0),
    tool(`${key}:down`, '↓', `Move section “${p.name}” down`, () => moveSection(song, p, 1), i === song.progressions.length - 1),
    tool(`${key}:rename`, '✎', `Rename section “${p.name}”`, () => renameSection(p)),
    tool(`${key}:del`, '✕', `Delete section “${p.name}”`, () => removeSection(p)),
  );
  return li;
}
