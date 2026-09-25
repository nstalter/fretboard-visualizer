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
export const canStep = (tuning, i, delta) => Math.abs(tuning[i] + delta - STANDARD[i]) <= MAX_OFFSET;

export function presetName(tuning) {
  for (const [name, p] of Object.entries(PRESETS)) {
    if (p.every((m, i) => m === tuning[i])) return name;
  }
  return '';
}
