const BASE = '/api'

async function request(method, path, body) {
    const opts = { method, headers: { 'Content-Type': 'application/json' } }
    if (body !== undefined) opts.body = JSON.stringify(body)
    const res = await fetch(`${BASE}${path}`, opts)
    if (!res.ok) {
        const text = await res.text()
        throw new Error(text || `HTTP ${res.status}`)
    }
    return res.json()
}

export const api = {
    submitJob: (filePath, operation, priority) =>
        request('POST', '/jobs', { file_path: filePath, operation, priority: Number(priority) }),

    submitBatch: (jobs) =>
        request('POST', '/batch', jobs),

    listJobs: () =>
        request('GET', '/jobs'),

    listFiles: (folderPath) =>
        request('GET', `/files?path=${encodeURIComponent(folderPath)}`),

    getStats: () =>
        request('GET', '/stats'),

    listWorkers: () =>
        request('GET', '/workers'),

    uploadFile: async (file) => {
        const form = new FormData()
        form.append('file', file)
        const r = await fetch('/api/upload', { method: 'POST', body: form })
        if (!r.ok) {
            const text = await r.text()
            throw new Error(text || `HTTP ${r.status}`)
        }
        return r.json()
    },
}
