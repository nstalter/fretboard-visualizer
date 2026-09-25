// renderFretboard rebuilds the SVG's children. It is pure: it holds no state.
// All per-string arrays are low→high; the high e is drawn at the top.

const NS = 'http://www.w3.org/2000/svg';
const FRETS = 22;
const NUT_X = 64;
const FRET_W = 52;
const STRING_GAP = 34;
const TOP = 44;
const WIDTH = NUT_X + FRETS * FRET_W + 12;
const HEIGHT = TOP + 5 * STRING_GAP + 22;
const INLAYS = [3, 5, 7, 9, 15, 17, 19, 21];
const DOT_R = 14;

const y = (s) => TOP + (5 - s) * STRING_GAP; // s is low→high
const fretX = (f) => NUT_X + (f - 0.5) * FRET_W;

function el(name, attrs = {}, text) {
  const node = document.createElementNS(NS, name);
  for (const [k, v] of Object.entries(attrs)) node.setAttribute(k, v);
  if (text !== undefined) node.textContent = text;
  return node;
}

// allTones, if given, is the [string][fret] map of chord-tone indices (-1 = not in the chord)
// from the server; every chord tone on the neck is then drawn faintly under the voicing.
// spotName(string, fret) is the note name shown while a spot is hovered (string low→high).
// onPick(string, fret), if given, is called when a spot is clicked (fret 0 = open).
export function renderFretboard(svg, { stringNames, voicing, chordTones, labelMode, allTones, spotName, onPick }) {
  svg.setAttribute('viewBox', `0 0 ${WIDTH} ${HEIGHT}`);
  svg.replaceChildren();

  const boardTop = y(5) - STRING_GAP / 2;
  const boardH = 5 * STRING_GAP + STRING_GAP;
  svg.append(el('rect', { class: 'board', x: NUT_X, y: boardTop, width: FRETS * FRET_W, height: boardH, rx: 4 }));

  // Inlays
  const midY = boardTop + boardH / 2;
  for (const f of INLAYS) svg.append(el('circle', { class: 'inlay', cx: fretX(f), cy: midY, r: 7 }));
  svg.append(el('circle', { class: 'inlay', cx: fretX(12), cy: midY - STRING_GAP, r: 7 }));
  svg.append(el('circle', { class: 'inlay', cx: fretX(12), cy: midY + STRING_GAP, r: 7 }));

  // Frets, nut and fret numbers
  for (let f = 1; f <= FRETS; f++) {
    const x = NUT_X + f * FRET_W;
    svg.append(el('line', { class: 'fret', x1: x, y1: boardTop, x2: x, y2: boardTop + boardH }));
    svg.append(el('text', { class: 'fret-num', x: fretX(f), y: boardTop - 10 }, String(f)));
  }
  svg.append(el('rect', { class: 'nut', x: NUT_X - 5, y: boardTop, width: 6, height: boardH }));

  // Strings, thicker toward the low string, with names from the current tuning
  for (let s = 0; s < 6; s++) {
    svg.append(el('line', { class: 'string', x1: NUT_X, y1: y(s), x2: NUT_X + FRETS * FRET_W, y2: y(s), 'stroke-width': 1 + (5 - s) * 0.35 }));
    svg.append(el('text', { class: 'string-name', x: 12, y: y(s) }, stringNames[s]));
  }

  if (allTones) appendAllTones(svg, allTones, voicing, chordTones, labelMode);

  if (!voicing) {
    svg.append(el('text', { class: 'empty', x: NUT_X + (FRETS * FRET_W) / 2, y: midY }, 'No playable shape for this chord/inversion with the current tuning and open-string limit'));
    return;
  }

  if (voicing.barre) {
    const { fret, from, to } = voicing.barre;
    svg.append(el('rect', {
      class: 'barre', x: fretX(fret) - DOT_R, y: y(to) - DOT_R,
      width: DOT_R * 2, height: y(from) - y(to) + DOT_R * 2, rx: DOT_R,
    }));
  }

  const label = (s) => {
    const tone = chordTones[voicing.tones[s]];
    return labelMode === 'intervals' ? tone.interval : tone.note;
  };

  for (let s = 0; s < 6; s++) {
    const fret = voicing.frets[s];
    const isRoot = voicing.tones[s] === 0;
    if (fret < 0) {
      svg.append(el('text', { class: 'muted', x: 40, y: y(s) }, '×'));
    } else if (fret === 0) {
      const g = el('g', { class: isRoot ? 'open root' : 'open' });
      g.append(el('circle', { cx: 40, cy: y(s), r: 11 }));
      g.append(el('text', { x: 40, y: y(s) }, label(s)));
      svg.append(g);
    } else {
      const g = el('g', { class: isRoot ? 'dot root' : 'dot' });
      g.append(el('circle', { cx: fretX(fret), cy: y(s), r: DOT_R }));
      g.append(el('text', { x: fretX(fret), y: y(s) }, label(s)));
      svg.append(g);
    }
  }

  if (onPick) appendHitTargets(svg, stringNames, spotName, onPick);
}

// appendAllTones draws a faint dot on every chord tone except where the voicing draws its own.
// At the nut the voicing's × or ○ occupies the spot, so a ring is drawn under a × without a label.
function appendAllTones(svg, allTones, voicing, chordTones, labelMode) {
  for (let s = 0; s < 6; s++) {
    for (let f = 0; f <= FRETS; f++) {
      const tone = allTones[s][f];
      if (tone < 0 || voicing?.frets[s] === f) continue;
      const nut = f === 0;
      const cx = nut ? 40 : fretX(f);
      const g = el('g', { class: `all-tone${nut ? ' nut' : ''}${tone === 0 ? ' root' : ''}` });
      g.append(el('circle', { cx, cy: y(s), r: nut ? 11 : DOT_R - 2 }));
      if (!(nut && voicing?.frets[s] < 0)) {
        g.append(el('text', { x: cx, y: y(s) }, labelMode === 'intervals' ? chordTones[tone].interval : chordTones[tone].note));
      }
      svg.append(g);
    }
  }
}

// appendHitTargets adds a clickable spot per string and fret, drawn on top of the board.
// Hovering a spot covers it with a bubble showing its note name; a name with two spellings
// ("C♯/D♭") is stacked on two lines. Fret 0 is the open-string marker area left of the nut.
function appendHitTargets(svg, stringNames, spotName, onPick) {
  for (let s = 0; s < 6; s++) {
    for (let f = 0; f <= FRETS; f++) {
      const nut = f === 0;
      const cx = nut ? 40 : fretX(f);
      const x = nut ? 26 : NUT_X + (f - 1) * FRET_W;
      const w = nut ? NUT_X - 5 - 26 : FRET_W;
      const name = spotName(s, f);
      const g = el('g', { class: 'hit' });
      g.append(el('title', {}, `${stringNames[s]} string, ${nut ? 'open' : `fret ${f}`}: ${name}`));
      g.append(el('circle', { class: 'ghost', cx, cy: y(s), r: nut ? 13 : DOT_R }));
      const parts = name.split('/');
      const size = parts.length === 1 ? (nut ? 11 : 13) : (nut ? 8.5 : 10);
      const gap = nut ? 5 : 6.5;
      parts.forEach((part, i) => {
        const dy = parts.length === 1 ? 0 : (i === 0 ? -gap : gap);
        g.append(el('text', { class: 'hover-label', x: cx, y: y(s) + dy, 'font-size': size }, part));
      });
      g.append(el('rect', { x, y: y(s) - STRING_GAP / 2, width: w, height: STRING_GAP }));
      g.addEventListener('click', () => onPick(s, f));
      svg.append(g);
    }
  }
}
