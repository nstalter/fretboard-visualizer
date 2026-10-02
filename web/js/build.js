// Build mode: place notes on the fretboard one string at a time. The server names the chord they
// spell and says whether it can be played; the board only ever holds a shape the server has accepted.
import * as api from './api.js';

const NONE = [-1, -1, -1, -1, -1, -1];
const INVERSION_NAMES = { root: '', any: '', '3rd': '3rd in bass', '5th': '5th in bass', '7th': '7th in bass' };

export const builder = {
  on: false,
  frets: [...NONE],   // low→high; -1 = not played
  chords: [],         // what the notes spell, best first (from /api/identify)
  pick: 0,            // which of chords is selected
  playable: false,
  issue: '',          // why the shape is not yet a playable chord
};
let seq = 0; // stale-response guard

// fingeringParam writes frets in the API's low→high form: "x32310", or dashed when a fret is above 9.
function fingeringParam(frets) {
  const tokens = frets.map((f) => (f < 0 ? 'x' : String(f)));
  return tokens.join(frets.some((f) => f > 9) ? '-' : '');
}

// identify asks the server about a shape and, if it is not ruled out, makes it the built shape. It
// throws the server's reason, leaving the built shape as it was, if no further notes could make the shape playable.
// It returns false if a newer call has taken over, in which case it changed nothing.
async function identify(frets, { tuning, openMax }) {
  const my = ++seq;
  if (frets.every((f) => f < 0)) {
    Object.assign(builder, { frets: [...NONE], chords: [], pick: 0, playable: false, issue: '' });
    return true;
  }
  const res = await api.identify({ fingering: fingeringParam(frets), tuning, openMax });
  if (my !== seq) return false;
  Object.assign(builder, { frets: [...frets], chords: res.chords, pick: 0, playable: res.playable, issue: res.issue });
  return true;
}

// place puts a note at a string (low→high) and fret, or removes it if that note is already there.
export function place(string, fret, settings) {
  const next = [...builder.frets];
  next[string] = next[string] === fret ? -1 : fret;
  return identify(next, settings);
}

// parseFingering reads the API's low→high fingering ("x32310", or dashed when a fret is above 9).
function parseFingering(s) {
  return (s.includes('-') ? s.split('-') : s.split('')).map((t) => (t === 'x' ? -1 : Number(t)));
}

// reset makes frets the built shape. The shape is cleared, and the reason thrown, if the server
// rules it out.
async function reset(frets, settings) {
  try {
    return await identify(frets, settings);
  } catch (err) {
    clear();
    throw err;
  }
}

// refresh re-checks the built shape after the tuning or open-string limit changed.
export const refresh = (settings) => reset(builder.frets, settings);

// load makes a shape (a fingering string, or null for none) the built shape, so it can be edited. If
// want ({root, quality}) names one of the chords the shape spells, that one is selected, so a G♯7 stays G♯7.
export async function load(fingering, settings, want) {
  const applied = await reset(fingering ? parseFingering(fingering) : NONE, settings);
  const i = want ? builder.chords.findIndex((c) => c.root === want.root && c.quality === want.quality) : -1;
  if (applied && i > 0) builder.pick = i;
}

export function clear() {
  seq++;
  Object.assign(builder, { frets: [...NONE], chords: [], pick: 0, playable: false, issue: '' });
}

// builtStep is the progression step for the selected chord, or null until the shape is a playable chord.
export function builtStep() {
  const c = builder.chords[builder.pick];
  if (!c || !builder.playable) return null;
  return { root: c.root, quality: c.quality, inversion: c.inversion, name: c.name, numeral: null, fingering: fingeringParam(builder.frets) };
}

// view is what the fretboard draws: the selected chord's own voicing, or while the notes are not
// a chord yet, their note names. In that case chordTones[0] is a placeholder, so no note shows as a root.
export function view(noteAt) {
  const c = builder.chords[builder.pick];
  if (c) return { voicing: c.voicing, chordTones: c.tones };
  const chordTones = [{}];
  const tones = builder.frets.map((fret, s) => {
    if (fret < 0) return -1;
    const note = noteAt(s, fret);
    chordTones.push({ note, interval: note });
    return chordTones.length - 1;
  });
  return { voicing: { frets: builder.frets, barre: null, tones }, chordTones };
}

function chordLabel(c) {
  const bass = INVERSION_NAMES[c.inversion];
  return bass ? `${c.name} (${bass})` : c.name;
}

// renderBar fills the build bar: the selected chord's name, the other names for the same notes, and a
// hint saying what is missing.
export function renderBar({ name, alts, hint, add }, { onPick }) {
  const c = builder.chords[builder.pick];
  name.textContent = c ? chordLabel(c) : '…';
  alts.replaceChildren(...(builder.chords.length > 1 ? builder.chords : []).map((alt, i) => {
    const b = document.createElement('button');
    b.textContent = chordLabel(alt);
    b.className = i === builder.pick ? 'on' : '';
    b.setAttribute('aria-pressed', String(i === builder.pick));
    b.addEventListener('click', () => onPick(i));
    return b;
  }));
  hint.textContent = builder.frets.every((f) => f < 0)
    ? 'Click the fretboard to place notes, one per string. Click a note again to remove it.'
    : builder.issue;
  add.disabled = !builtStep();
}

export function pickChord(i) {
  builder.pick = i;
}
