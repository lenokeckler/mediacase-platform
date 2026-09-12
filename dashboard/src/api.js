// Cliente HTTP del dashboard. En desarrollo (npm run dev) Vite manda /api al coordinador;
// en producción el propio coordinador sirve el dashboard y atiende /api.
const BASE = '/api'

async function request(method, path, body) {
    const opts = { method, headers: { 'Content-Type': 'application/json' } }
    if (body !== undefined) opts.body = JSON.stringify(body)
    const res = await fetch(`${BASE}${path}`, opts)
    if (!res.ok) {
        const text = await res.text()
        throw new Error(text || `HTTP ${res.status}`)
    }
    if (res.status === 204) return null
    return res.json()
}

export const api = {
    // ── Casos (la unidad de trabajo) ──
    submitCase: (name, priority, files) =>
        request('POST', '/cases', { name, priority: Number(priority), files }),
    listCases: (status = '') =>
        request('GET', `/cases${status ? `?status=${encodeURIComponent(status)}` : ''}`),
    getCase: (id) => request('GET', `/cases/${id}`),
    getCaseReport: (id) => request('GET', `/cases/${id}/report`),
    cancelCase: (id) => request('POST', `/cases/${id}/cancel`),

    // ── Entradas (bucket dataset en MinIO) ──
    listDataset: (prefix = '') => request('GET', `/dataset?prefix=${encodeURIComponent(prefix)}`),
    uploadFiles: async (fileList) => {
        const form = new FormData()
        for (const f of fileList) form.append('file', f)
        const r = await fetch(`${BASE}/upload`, { method: 'POST', body: form })
        if (!r.ok) throw new Error((await r.text()) || `HTTP ${r.status}`)
        return r.json() // { keys: [...] }
    },

    // ── Sub-tareas sueltas, workers, estadísticas ──
    submitJob: (filePath, operation, priority) =>
        request('POST', '/jobs', { file_path: filePath, operation, priority: Number(priority) }),
    listJobs: () => request('GET', '/jobs'),
    getStats: () => request('GET', '/stats'),
    listWorkers: () => request('GET', '/workers'),

    // ── Compartir node-1: URLs de la LAN y túnel a internet ──
    getCatalog: () => request('GET', '/catalog'),
    getShare: () => request('GET', '/share'),
    startTunnel: () => request('POST', '/tunnel'),
    stopTunnel: () => request('DELETE', '/tunnel'),
}

// ── Utilidades compartidas por los componentes ──

export const CASE_STATUS_LABEL = {
    queued: 'en cola',
    processing: 'procesando',
    retrying: 'reintentando',
    completed: 'completado',
    partially_completed: 'parcial',
    failed: 'fallido',
    cancelled: 'cancelado',
}

export const JOB_STATUS_LABEL = {
    pending: 'pendiente',
    assigned: 'asignada',
    running: 'en ejecución',
    completed: 'completada',
    failed: 'fallida',
    cancelled: 'cancelada',
}

// Operaciones: nombre corto, verbo para tablas y descripción para el formulario.
export const OPERATION_LABEL = {
    convert: 'convertir video',
    convert_audio: 'convertir audio',
    extract_audio: 'extraer audio',
    thumbnail: 'miniatura',
    metadata: 'metadatos',
}
export const OPERATION_HELP = {
    convert: 'Transcodifica el video a otro contenedor/códec (MP4 H.264, MKV o WebM VP9).',
    convert_audio: 'Convierte el audio a otro formato (FLAC sin pérdida, MP3, WAV, AAC u OGG).',
    extract_audio: 'Saca solo la pista de audio del video.',
    thumbnail: 'Genera una imagen pequeña: primer fotograma del video/imagen o forma de onda del audio.',
    metadata: 'Consulta con ffprobe: duración, códecs, resolución, bitrate y etiquetas → JSON.',
}

// Catálogo por defecto (misma tabla que internal/cases/router.go). El coordinador sirve la versión
// autoritativa en GET /catalog; esto solo cubre el arranque y el caso sin red.
export const DEFAULT_CATALOG = {
    ops_by_type: {
        video: ['convert', 'extract_audio', 'thumbnail', 'metadata'],
        audio: ['convert_audio', 'thumbnail', 'metadata'],
        image: ['thumbnail', 'metadata'],
    },
    targets_by_op: {
        convert: ['mp4', 'mkv', 'webm'],
        extract_audio: ['mp3', 'wav', 'flac', 'aac'],
        convert_audio: ['flac', 'mp3', 'wav', 'aac', 'ogg'],
        thumbnail: ['jpg', 'png', 'webp'],
        metadata: ['json'],
    },
    pool_by_op: { convert: 'video', extract_audio: 'video', convert_audio: 'audio', thumbnail: 'metadata', metadata: 'metadata' },
    thumbnail_widths: [320, 640, 1280],
}
export const OPS_BY_TYPE = DEFAULT_CATALOG.ops_by_type

// "mkv → MP4": cómo se muestra una sub-tarea en tablas y reportes.
export function opArrow(job) {
    const src = extOf(job.file_path || job.file || '')
    const dst = job.target || ''
    if (!dst) return OPERATION_LABEL[job.operation] || job.operation
    return `${src || '?'} → ${dst.toUpperCase()}`
}
export function extOf(name) { return (name || '').toLowerCase().split('.').pop() }

const EXT_TYPE = {
    mp4: 'video', mkv: 'video', avi: 'video', mov: 'video', webm: 'video', m4v: 'video', flv: 'video', wmv: 'video', ts: 'video', mts: 'video', '3gp': 'video', mpg: 'video', mpeg: 'video',
    mp3: 'audio', wav: 'audio', flac: 'audio', aac: 'audio', ogg: 'audio', m4a: 'audio', opus: 'audio', wma: 'audio', aiff: 'audio', aif: 'audio', dsf: 'audio', dff: 'audio',
    jpg: 'image', jpeg: 'image', png: 'image', gif: 'image', webp: 'image', bmp: 'image', tif: 'image', tiff: 'image',
}
export const ACCEPT_EXTENSIONS = Object.keys(EXT_TYPE).map(e => '.' + e).join(',')

export function fileTypeOf(name) {
    const ext = (name || '').toLowerCase().split('.').pop()
    return EXT_TYPE[ext] || null
}

export function isTerminal(status) {
    return ['completed', 'partially_completed', 'failed', 'cancelled'].includes(status)
}

export function fmtBytes(n) {
    if (n == null) return '—'
    if (n < 1024) return `${n} B`
    if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)} KB`
    if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`
    return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`
}

export function fmtSeconds(s) {
    if (s == null) return '—'
    if (s < 60) return `${s.toFixed(s < 10 ? 1 : 0)} s`
    const m = Math.floor(s / 60)
    return `${m} min ${Math.round(s - m * 60)} s`
}

export function fmtTime(iso) {
    if (!iso) return '—'
    return new Date(iso).toLocaleTimeString('es-CR', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}
