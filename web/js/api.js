// One fetch wrapper per endpoint. A non-2xx response throws Error(body.error).

async function request(url, init) {
  const res = await fetch(url, init);
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(body.error || `${res.status} ${res.statusText}`);
  return body;
}

function get(path, params) {
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== null && v !== undefined && v !== '') qs.set(k, v);
  }
  return request(`${path}?${qs}`);
}

export const qualities = () => request('/api/qualities');

export const voicings = ({ root, quality, inversion, tuning, openMax, near, at }) =>
  get('/api/voicings', { root, quality, inversion, tuning, openMax, near, at });

export const diatonic = ({ key, mode }) => get('/api/diatonic', { key, mode });

export const progression = ({ tuning, openMax, steps }) =>
  request('/api/progression', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ tuning, openMax, steps }),
  });
