import * as api from './api.js';
import { renderFretboard } from './fretboard.js';
import { PRESETS, STANDARD, noteName, pitchClassName, tuningParam, canStep, presetName } from './tuning.js';
import { renderSteps, Player } from './progression.js';

const ROOTS = ['C', 'C♯', 'D♭', 'D', 'D♯', 'E♭', 'E', 'F', 'F♯', 'G♭', 'G', 'G♯', 'A♭', 'A', 'A♯', 'B♭', 'B'];
const MODES = ['ionian', 'dorian', 'phrygian', 'lydian', 'mixolydian', 'aeolian', 'locrian'];
const INVERSION_LABELS = { any: 'Any', root: 'Root', '3rd': '3rd', '5th': '5th', '7th': '7th' };

const state = {
  qualities: [], tuning: [...STANDARD], labelMode: 'notes',
  openMax: 5,                          // highest fret at which a shape may still use an open string; 22 = no limit
  showAll: false,                      // draw every chord tone on the neck
  explorer: { chooser: 'mode', root: 'C', quality: 'maj',
              key: 'C', mode: 'ionian', degree: 1, modeQuality: 'diatonic', diatonic: [],
              inversion: 'any', chord: null, fretboard: null, voicings: [], index: 0,
              lastFingering: null },   // reference for `near`; null after an empty result, so the next change shows index 0
  steps: [],                           // {root, quality, inversion, name, numeral|null, fingering}
  selectedStep: null,                  // the step object itself, not an index, so it survives reorders
  playback: { bpm: 90, beatsPerChord: 4 },
};

const $ = (id) => document.getElementById(id);
const player = new Player(() => (60000 / state.playback.bpm) * state.playback.beatsPerChord, () => stepPlayback(1));
let seq = 0; // stale-response guard for loadVoicings
let settingSeq = 0; // the same idea for applySettingChange

// ---- helpers ----

function option(value, text = value) {
  const o = document.createElement('option');
  o.value = value;
  o.textContent = text;
  return o;
}

// setSelect sets a select's value, adding the option first if needed (e.g. a F♯♯ root from a mode).
function setSelect(select, value) {
  if (![...select.options].some((o) => o.value === value)) select.append(option(value));
  select.value = value;
}

function showError(err) {
  $('status').textContent = err ? err.message : '';
}

function qualityInfo(id) {
  return state.qualities.find((q) => q.id === id);
}

// currentChoice is the root and quality the explorer's pickers currently select.
function currentChoice() {
  const e = state.explorer;
  if (e.chooser === 'type') return { root: e.root, quality: e.quality };
  const d = e.diatonic[e.degree - 1];
  if (!d) return null;
  return { root: d.root, quality: e.modeQuality === 'diatonic' ? d.quality : e.modeQuality };
}

function currentVoicing() {
  const e = state.explorer;
  return e.voicings[e.index] ?? null;
}

// spotName is the note sounded at a string (low→high) and fret: the chord's own spelling for a
// chord tone, otherwise both enharmonic names ("C♯/D♭").
function spotName(s, f) {
  const e = state.explorer;
  const tone = e.fretboard?.[s]?.[f] ?? -1;
  return tone >= 0 ? e.chord.tones[tone].note : pitchClassName(state.tuning[s] + f);
}

function pause() {
  player.pause();
  renderPlayButton();
}

// ---- server calls ----

// loadVoicings fetches the current chord's shapes and picks the one nearest `near`,
// or the exact `exact` fingering when present. With `at` ("string:fret") the server picks
// a shape playing that spot. Returns the response, or null if it was stale or failed.
async function loadVoicings({ near = state.explorer.lastFingering, exact = null, at = null } = {}) {
  const e = state.explorer;
  const choice = currentChoice();
  if (!choice) return null;
  const my = ++seq;
  let data = null;
  try {
    data = await api.voicings({ ...choice, inversion: e.inversion, tuning: tuningParam(state.tuning), openMax: state.openMax, near, at });
    if (my !== seq) return null;
    e.chord = data.chord;
    e.fretboard = data.fretboard;
    e.voicings = data.voicings;
    let index = data.nearestIndex ?? 0;
    if (exact) {
      const j = data.voicings.findIndex((v) => v.fingering === exact);
      if (j >= 0) index = j;
    }
    e.index = index;
    e.lastFingering = data.voicings.length ? data.voicings[index].fingering : null;
    showError(null);
  } catch (err) {
    if (my === seq) showError(err);
    data = null;
  }
  render();
  return data;
}

async function loadDiatonic() {
  const e = state.explorer;
  try {
    const data = await api.diatonic({ key: e.key, mode: e.mode });
    e.diatonic = data.chords;
  } catch (err) {
    showError(err);
  }
}

// ---- explorer changes ----

// onChordChange handles a root, quality, inversion, key, mode, degree or chooser change.
async function onChordChange({ refreshDiatonic = false } = {}) {
  pause();
  state.selectedStep = null; // D13: a chord change leaves the loaded step unchanged
  const e = state.explorer;
  if (refreshDiatonic || (e.chooser === 'mode' && e.diatonic.length === 0)) await loadDiatonic();
  const choice = currentChoice();
  const offered = choice && qualityInfo(choice.quality)?.inversions;
  if (offered && !offered.includes(e.inversion)) e.inversion = 'any';
  await loadVoicings();
}

function stepShape(delta) {
  pause();
  const e = state.explorer;
  const n = e.voicings.length;
  if (!n) return;
  e.index = (e.index + delta + n) % n;
  e.lastFingering = e.voicings[e.index].fingering;
  if (state.selectedStep) state.selectedStep.fingering = e.lastFingering;
  render();
}

// pickSpot jumps to a shape of the current chord that plays the clicked spot
// (string low→high, fret 0 = open), falling back to the shape nearest that fret.
async function pickSpot(string, fret) {
  pause();
  const data = await loadVoicings({ at: `${string}:${fret}` });
  if (!data || !data.voicings.length) return;
  if (state.selectedStep) {
    state.selectedStep.fingering = state.explorer.lastFingering;
    render();
  }
}

// ---- tuning and open-string limit ----

// applySettingChange recalculates every step's shape for the current tuning and open-string
// limit, then shows the chord nearest the selected step (or the current shape). A newer call
// supersedes an older one, and shapes go to the step objects that were sent, not to list positions,
// so adding, moving or deleting a step while the request is in flight cannot misplace them.
async function applySettingChange() {
  pause();
  const my = ++settingSeq;
  const steps = [...state.steps];
  let failure = null;
  if (steps.length) {
    try {
      const { shapes } = await api.progression({
        tuning: tuningParam(state.tuning),
        openMax: state.openMax,
        steps: steps.map(({ root, quality, inversion }) => ({ root, quality, inversion })),
      });
      if (my !== settingSeq) return;
      shapes.forEach((shape, i) => { steps[i].fingering = shape; });
    } catch (err) {
      if (my !== settingSeq) return;
      failure = err;
    }
  }
  const sel = state.selectedStep;
  if (sel) {
    state.explorer.lastFingering = sel.fingering;
    await loadVoicings({ near: sel.fingering, exact: sel.fingering });
  } else {
    await loadVoicings();
  }
  // loadVoicings clears the status line on success; keep the recalculation failure visible.
  if (failure && my === settingSeq && !$('status').textContent) {
    showError(new Error(`Could not recalculate the progression: ${failure.message}`));
  }
}

async function onTuningChange(tuning) {
  state.tuning = tuning;
  await applySettingChange();
}

async function onOpenMaxChange(openMax) {
  state.openMax = openMax;
  await applySettingChange();
}

// ---- progression ----

function addStep() {
  pause();
  const e = state.explorer;
  const v = currentVoicing();
  if (!v || !e.chord) return;
  let numeral = null;
  if (e.chooser === 'mode') numeral = e.diatonic[e.degree - 1]?.numeral ?? null;
  else if (state.selectedStep) numeral = state.selectedStep.numeral;
  state.steps.push({
    root: e.chord.root, quality: e.chord.quality, inversion: e.inversion,
    name: e.chord.name, numeral, fingering: v.fingering,
  });
  render();
}

// loadStep selects a step and shows it in the explorer. It does not pause playback.
async function loadStep(step) {
  const e = state.explorer;
  state.selectedStep = step;
  e.chooser = 'type';
  e.root = step.root;
  e.quality = step.quality;
  e.inversion = step.inversion;
  e.lastFingering = step.fingering;
  render();
  await loadVoicings({ near: step.fingering, exact: step.fingering });
}

function stepPlayback(delta) {
  const n = state.steps.length;
  if (!n) return;
  const i = state.steps.indexOf(state.selectedStep);
  const next = i < 0 ? 0 : (i + delta + n) % n;
  loadStep(state.steps[next]);
}

function moveStep(step, delta) {
  pause();
  const i = state.steps.indexOf(step);
  const j = i + delta;
  if (j < 0 || j >= state.steps.length) return;
  [state.steps[i], state.steps[j]] = [state.steps[j], state.steps[i]];
  render();
}

function deleteStep(step) {
  pause();
  state.steps.splice(state.steps.indexOf(step), 1);
  if (state.selectedStep === step) state.selectedStep = null;
  render();
}

// ---- rendering ----

function renderPlayButton() {
  $('play').textContent = player.playing ? 'Pause' : 'Play';
}

function render() {
  const e = state.explorer;
  $('preset').value = presetName(state.tuning);

  $('chooser-type').classList.toggle('on', e.chooser === 'type');
  $('chooser-mode').classList.toggle('on', e.chooser === 'mode');
  $('type-pickers').hidden = e.chooser !== 'type';
  $('mode-pickers').hidden = e.chooser !== 'mode';
  $('degrees').hidden = e.chooser !== 'mode';
  setSelect($('root'), e.root);
  $('quality').value = e.quality;
  $('key').value = e.key;
  $('mode').value = e.mode;
  $('mode-quality').value = e.modeQuality;

  const degrees = $('degrees');
  degrees.replaceChildren();
  e.diatonic.forEach((d) => {
    const b = document.createElement('button');
    b.className = d.degree === e.degree ? 'on' : '';
    b.innerHTML = `<span class="numeral"></span> <span class="dname"></span>`;
    b.querySelector('.numeral').textContent = d.numeral;
    b.querySelector('.dname').textContent = d.name;
    b.addEventListener('click', () => { e.degree = d.degree; onChordChange(); });
    degrees.append(b);
  });

  const choice = currentChoice();
  const inv = $('inversion');
  inv.replaceChildren(...(qualityInfo(choice?.quality)?.inversions ?? ['any']).map((i) => option(i, INVERSION_LABELS[i])));
  inv.value = e.inversion;

  $('label-notes').classList.toggle('on', state.labelMode === 'notes');
  $('label-intervals').classList.toggle('on', state.labelMode === 'intervals');
  $('show-all').classList.toggle('on', state.showAll);
  $('show-all').setAttribute('aria-pressed', String(state.showAll));

  const v = currentVoicing();
  $('chord-name').textContent = e.chord ? e.chord.name : '…';
  $('counter').textContent = `${e.voicings.length ? e.index + 1 : 0} / ${e.voicings.length}`;
  $('prev-shape').disabled = $('next-shape').disabled = e.voicings.length < 2;
  $('add-step').disabled = !v;

  renderFretboard($('fretboard'), {
    stringNames: state.tuning.map(noteName),
    voicing: v,
    chordTones: e.chord?.tones ?? [],
    labelMode: state.labelMode,
    allTones: state.showAll ? e.fretboard : null,
    spotName,
    onPick: pickSpot,
    onTune: (s, delta) => onTuningChange(state.tuning.map((m, k) => (k === s ? m + delta : m))),
    canTune: (s, delta) => canStep(state.tuning, s, delta),
  });

  renderSteps($('steps'), state.steps, state.selectedStep, {
    onSelect: (step) => { loadStep(step); player.restart(); },
    onMove: moveStep,
    onDelete: deleteStep,
  });
  $('steps-empty').hidden = state.steps.length > 0;
  const hasSteps = state.steps.length > 0;
  $('play').disabled = $('play-prev').disabled = $('play-next').disabled = !hasSteps;
  renderPlayButton();
}

// ---- wiring ----

function wire() {
  const e = state.explorer;

  for (const r of ROOTS) { $('root').append(option(r)); $('key').append(option(r)); }
  for (const m of MODES) $('mode').append(option(m, m[0].toUpperCase() + m.slice(1)));
  $('mode-quality').append(option('diatonic', 'Diatonic'));
  for (const q of state.qualities) {
    $('quality').append(option(q.id, q.label));
    $('mode-quality').append(option(q.id, q.label));
  }
  $('preset').append(option('', 'Custom'));
  for (const name of Object.keys(PRESETS)) $('preset').append(option(name));

  for (const b of document.querySelectorAll('[data-chooser]')) {
    b.addEventListener('click', () => {
      if (e.chooser === b.dataset.chooser) return;
      e.chooser = b.dataset.chooser;
      onChordChange();
    });
  }
  $('root').addEventListener('change', (ev) => { e.root = ev.target.value; onChordChange(); });
  $('quality').addEventListener('change', (ev) => { e.quality = ev.target.value; onChordChange(); });
  $('key').addEventListener('change', (ev) => { e.key = ev.target.value; onChordChange({ refreshDiatonic: true }); });
  $('mode').addEventListener('change', (ev) => { e.mode = ev.target.value; onChordChange({ refreshDiatonic: true }); });
  $('mode-quality').addEventListener('change', (ev) => { e.modeQuality = ev.target.value; onChordChange(); });
  $('inversion').addEventListener('change', (ev) => { e.inversion = ev.target.value; onChordChange(); });

  for (const b of document.querySelectorAll('[data-label]')) {
    b.addEventListener('click', () => { pause(); state.labelMode = b.dataset.label; render(); });
  }
  $('show-all').addEventListener('click', () => { pause(); state.showAll = !state.showAll; render(); });

  $('open-max').value = state.openMax;
  $('open-max').addEventListener('change', (ev) => {
    const n = Math.round(Number(ev.target.value));
    const openMax = ev.target.value.trim() === '' || Number.isNaN(n) ? state.openMax : Math.min(22, Math.max(0, n));
    ev.target.value = openMax;
    if (openMax !== state.openMax) onOpenMaxChange(openMax);
  });
  $('prev-shape').addEventListener('click', () => stepShape(-1));
  $('next-shape').addEventListener('click', () => stepShape(1));
  $('add-step').addEventListener('click', addStep);

  $('preset').addEventListener('change', (ev) => {
    if (PRESETS[ev.target.value]) onTuningChange([...PRESETS[ev.target.value]]);
  });

  $('play').addEventListener('click', () => {
    if (player.playing) {
      player.pause();
    } else {
      if (!state.selectedStep && state.steps.length) loadStep(state.steps[0]);
      player.play();
    }
    renderPlayButton();
  });
  $('play-prev').addEventListener('click', () => { stepPlayback(-1); player.restart(); });
  $('play-next').addEventListener('click', () => { stepPlayback(1); player.restart(); });
  $('bpm').addEventListener('change', (ev) => {
    state.playback.bpm = Math.min(300, Math.max(20, Number(ev.target.value) || 90));
    ev.target.value = state.playback.bpm;
    player.restart();
  });
  $('beats').addEventListener('change', (ev) => {
    state.playback.beatsPerChord = Math.min(16, Math.max(1, Math.round(Number(ev.target.value)) || 4));
    ev.target.value = state.playback.beatsPerChord;
    player.restart();
  });
}

async function start() {
  try {
    state.qualities = (await api.qualities()).qualities;
  } catch (err) {
    showError(err);
    return;
  }
  wire();
  render();
  await loadDiatonic();
  await loadVoicings({ near: null });
}

start();
