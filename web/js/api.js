// One fetch wrapper per endpoint. A non-2xx response throws Error(body.error).

async function request(url, init) {
  const res = await fetch(url, init);
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw Object.assign(new Error(body.error || `${res.status} ${res.statusText}`), { status: res.status });
  return body;
}

// send is request with a method and, when given, a JSON body.
function send(method, url, body) {
  const init = { method };
  if (body !== undefined) {
    init.headers = { 'Content-Type': 'application/json' };
    init.body = JSON.stringify(body);
  }
  return request(url, init);
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

// Accounts and the saved-song library. A 401 means signed out (the thrown Error has .status).

export const me = () => request('/api/me', { cache: 'no-store' });
export const logout = () => send('POST', '/auth/logout');

export const library = () => request('/api/library', { cache: 'no-store' });

export const createFolder = (name) => send('POST', '/api/folders', { name });
export const renameFolder = (id, name) => send('PATCH', `/api/folders/${id}`, { name });
export const deleteFolder = (id) => send('DELETE', `/api/folders/${id}`);

// folderId: a number files the song there; null unfiles it; undefined leaves it alone (or, on create, unfiled).
export const createSong = ({ name, folderId }) => send('POST', '/api/songs', { name, folderId });
export const updateSong = (id, fields) => send('PATCH', `/api/songs/${id}`, fields);
export const deleteSong = (id) => send('DELETE', `/api/songs/${id}`);

export const createProgression = (songId, { name, data }) =>
  send('POST', `/api/songs/${songId}/progressions`, { name, data });
export const reorderProgressions = (songId, ids) =>
  send('PUT', `/api/songs/${songId}/progressions/order`, { ids });
export const getProgression = (id) => request(`/api/progressions/${id}`, { cache: 'no-store' });
export const updateProgression = (id, fields) => send('PUT', `/api/progressions/${id}`, fields);
export const deleteProgression = (id) => send('DELETE', `/api/progressions/${id}`);
