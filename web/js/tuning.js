// Tuning presets as MIDI numbers, low→high.

export const STANDARD = [40, 45, 50, 55, 59, 64];
export const MAX_OFFSET = 5;

export const PRESETS = {
  'Standard': STANDARD,
  'Drop D': [38, 45, 50, 55, 59, 64],
  'Half-step down': [39, 44, 49, 54, 58, 63],
  'DADGAD': [38, 45, 50, 55, 57, 62],
  'Open G': [38, 43, 50, 55, 59, 62],
  'Open D': [38, 45, 50, 54, 57, 62],
};

const NAMES = ['C', 'C♯', 'D', 'E♭', 'E', 'F', 'F♯', 'G', 'A♭', 'A', 'B♭', 'B'];

export const noteName = (midi) => NAMES[midi % 12];

// Both enharmonic names of a pitch class, for notes outside the chord: "C♯/D♭". Naturals have one.
const PITCH_CLASSES = ['C', 'C♯/D♭', 'D', 'D♯/E♭', 'E', 'F', 'F♯/G♭', 'G', 'G♯/A♭', 'A', 'A♯/B♭', 'B'];
export const pitchClassName = (midi) => PITCH_CLASSES[midi % 12];
export const pitchName = (midi) => `${noteName(midi)}${Math.floor(midi / 12) - 1}`;
export const tuningParam = (tuning) => tuning.map(pitchName).join(',');

// parseTuningParam is the inverse of tuningParam: "E2,A2,D3,G3,B3,E4" → MIDI numbers low→high, or null if malformed.
const SEMITONES = { C: 0, D: 2, E: 4, F: 5, G: 7, A: 9, B: 11 };
export function parseTuningParam(s) {
  const parts = String(s).split(',');
  if (parts.length !== 6) return null;
  const midi = [];
  for (const part of parts) {
    const m = /^([A-G])([#♯]*|[b♭]*)(\d+)$/.exec(part.trim());
    if (!m) return null;
    const accidental = m[2] === '' ? 0 : (m[2][0] === '#' || m[2][0] === '♯' ? 1 : -1) * m[2].length;
    midi.push((Number(m[3]) + 1) * 12 + SEMITONES[m[1]] + accidental);
  }
  return midi;
}

export const canStep = (tuning, i, delta) => Math.abs(tuning[i] + delta - STANDARD[i]) <= MAX_OFFSET;

export function presetName(tuning) {
  for (const [name, p] of Object.entries(PRESETS)) {
    if (p.every((m, i) => m === tuning[i])) return name;
  }
  return '';
}
