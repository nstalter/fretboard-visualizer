// Step list rendering and the playback timer.

export function renderSteps(list, steps, selected, { onSelect, onMove, onDelete }) {
  list.replaceChildren();
  steps.forEach((step, i) => {
    const li = document.createElement('li');
    li.className = step === selected ? 'step selected' : 'step';

    const name = document.createElement('button');
    name.className = 'step-name';
    name.textContent = step.numeral ? `${step.name} · ${step.numeral}` : step.name;
    name.title = step.fingering ?? 'No playable shape with the current tuning and open-string limit';
    name.addEventListener('click', () => onSelect(step));

    const up = iconButton('↑', 'Move up', i === 0, () => onMove(step, -1));
    const down = iconButton('↓', 'Move down', i === steps.length - 1, () => onMove(step, 1));
    const del = iconButton('✕', 'Delete', false, () => onDelete(step));
    li.append(name, up, down, del);
    list.append(li);
  });
}

function iconButton(text, label, disabled, onClick) {
  const b = document.createElement('button');
  b.className = 'icon';
  b.textContent = text;
  b.title = label;
  b.setAttribute('aria-label', label);
  b.disabled = disabled;
  b.addEventListener('click', onClick);
  return b;
}

// Player calls onTick every intervalMs() milliseconds while playing.
export class Player {
  constructor(intervalMs, onTick) {
    this.intervalMs = intervalMs;
    this.onTick = onTick;
    this.timer = null;
  }
  get playing() {
    return this.timer !== null;
  }
  play() {
    if (this.playing) return;
    this.schedule();
  }
  pause() {
    clearTimeout(this.timer);
    this.timer = null;
  }
  // restart keeps playing but starts a fresh interval (after a jump or tempo change).
  restart() {
    if (!this.playing) return;
    this.pause();
    this.schedule();
  }
  schedule() {
    this.timer = setTimeout(() => {
      this.schedule();
      this.onTick();
    }, this.intervalMs());
  }
}
