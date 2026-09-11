import { useState, useRef } from 'react'
import { api } from '../api'
import styles from './SubmitJobPanel.module.css'

const DEFAULT_PRIORITY = 5
const PRIORITY_OPTIONS = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]

const VIDEO_EXTS = new Set(['mp4', 'mkv', 'avi', 'mov', 'webm'])
const MEDIA_EXTS = new Set(['mp4', 'mkv', 'avi', 'mov', 'webm', 'mp3', 'wav', 'aac', 'flac', 'ogg'])

const VIDEO_OPS = [
    { value: 'convert',       label: 'Convert to MP4' },
    { value: 'extract_audio', label: 'Extract Audio (MP3)' },
    { value: 'thumbnail',     label: 'Generate Thumbnail' },
]

const AUDIO_OPS = [
    { value: 'extract_audio', label: 'Re-encode to MP3' },
    { value: 'convert_audio', label: 'Convert to WAV' },
    { value: 'thumbnail',     label: 'Generate Waveform' },
]

function getType(name) {
    const ext = name.toLowerCase().split('.').pop()
    return VIDEO_EXTS.has(ext) ? 'video' : 'audio'
}

function getExt(name) {
    return name.toLowerCase().split('.').pop()
}

function getOps(name, type) {
    const ext = getExt(name)
    if (type === 'video') {
        return VIDEO_OPS.filter(op => !(op.value === 'convert' && ext === 'mp4'))
    }
    return AUDIO_OPS.filter(op => {
        if (op.value === 'extract_audio' && ext === 'mp3') return false
        if (op.value === 'convert_audio' && ext === 'wav') return false
        return true
    })
}

function fmtSize(bytes) {
    if (!bytes) return '—'
    if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KB`
    if (bytes < 1073741824) return `${(bytes / 1048576).toFixed(1)} MB`
    return `${(bytes / 1073741824).toFixed(2)} GB`
}

const UPLOAD_DEST = '/app/dataset/files'

export default function SubmitJobPanel() {
    const inputRef = useRef(null)

    // Each entry: { id, name, size, type, operation, file }
    const [files, setFiles] = useState([])
    const [phase, setPhase] = useState('idle')   // idle | uploading | busy | ok | err
    const [submitted, setSubmitted] = useState(0)
    const [errMsg, setErrMsg] = useState('')

    function handleFileChange(e) {
        const picked = Array.from(e.target.files)
            .filter(f => {
                const ext = f.name.toLowerCase().split('.').pop()
                return MEDIA_EXTS.has(ext)
            })
            .map(f => {
                const type = getType(f.name)
                const ops = getOps(f.name, type)
                return {
                    id: `${f.name}-${f.size}-${Date.now()}`,
                    name: f.name,
                    size: f.size,
                    type,
                    operation: ops[0].value,
                    priority: DEFAULT_PRIORITY,
                    file: f,
                }
            })
        setFiles(prev => {
            const existing = new Set(prev.map(f => f.name))
            return [...prev, ...picked.filter(f => !existing.has(f.name))]
        })
        e.target.value = ''
    }

    function removeFile(id) {
        setFiles(prev => prev.filter(f => f.id !== id))
    }

    function updateOp(id, operation) {
        setFiles(prev => prev.map(f => f.id === id ? { ...f, operation } : f))
    }

    function updatePriority(id, priority) {
        setFiles(prev => prev.map(f => f.id === id ? { ...f, priority: Number(priority) } : f))
    }

    async function handleSubmit() {
        if (!files.length) return
        setPhase('uploading')
        setErrMsg('')
        try {
            // Upload all files to the coordinator first, get their server paths back.
            const uploaded = await Promise.all(
                files.map(async f => {
                    const { path } = await api.uploadFile(f.file)
                    return { file_path: path, operation: f.operation, priority: f.priority }
                })
            )

            setPhase('busy')

            let count
            if (uploaded.length === 1) {
                await api.submitJob(uploaded[0].file_path, uploaded[0].operation, uploaded[0].priority)
                count = 1
            } else {
                const result = await api.submitBatch(uploaded)
                count = Array.isArray(result) ? result.length : uploaded.length
            }

            setSubmitted(count)
            setPhase('ok')
            setFiles([])
        } catch (err) {
            setErrMsg(err.message)
            setPhase('err')
        }
    }

    const busy = phase === 'uploading' || phase === 'busy'

    return (
        <div className={styles.panel}>
            <div className={styles.panelHeader}>
                <h3 className={styles.title}>Manual Submission</h3>
                <p className={styles.desc}>Pick one or more files — they will be uploaded to the server and queued automatically.</p>
            </div>

            {/* ── File picker ── */}
            <div className={styles.pickerRow}>
                <input
                    ref={inputRef}
                    type="file"
                    multiple
                    accept=".mp4,.mkv,.avi,.mov,.webm,.mp3,.wav,.aac,.flac,.ogg"
                    className={styles.hiddenInput}
                    onChange={handleFileChange}
                />
                <button
                    className={styles.addBtn}
                    onClick={() => inputRef.current?.click()}
                    disabled={busy}
                >
                    📎 Add Files
                </button>
                {files.length > 0 && (
                    <span className={styles.fileCount}>
                        {files.length} file{files.length !== 1 ? 's' : ''} selected
                    </span>
                )}
            </div>

            {/* ── File list with per-file operation ── */}
            {files.length > 0 && (
                <>
                    <div className={styles.fileTable}>
                        <div className={styles.fileTableHead}>
                            <span>Type</span>
                            <span>File</span>
                            <span>Operation</span>
                            <span>Priority</span>
                            <span />
                        </div>
                        {files.map(f => {
                            const ops = getOps(f.name, f.type)
                            return (
                                <div key={f.id} className={styles.fileTableRow}>
                                    <span
                                        className={styles.typeBadge}
                                        style={f.type === 'video'
                                            ? { background: '#1e3a5f', color: '#60a5fa' }
                                            : { background: '#1c2e1c', color: '#4ade80' }}
                                    >
                                        {f.type}
                                    </span>
                                    <span className={styles.fileName} title={`${UPLOAD_DEST}/${f.name}`}>
                                        {f.name}
                                        <span className={styles.fileSize}>{fmtSize(f.size)}</span>
                                        <span className={styles.serverPath}>{UPLOAD_DEST}/{f.name}</span>
                                    </span>
                                    <select
                                        className={styles.opSelect}
                                        value={f.operation}
                                        onChange={e => updateOp(f.id, e.target.value)}
                                        disabled={busy}
                                    >
                                        {ops.map(op => (
                                            <option key={op.value} value={op.value}>{op.label}</option>
                                        ))}
                                    </select>
                                    <select
                                        className={styles.opSelect}
                                        value={f.priority}
                                        onChange={e => updatePriority(f.id, e.target.value)}
                                        disabled={busy}
                                    >
                                        {PRIORITY_OPTIONS.map(p => (
                                            <option key={p} value={p}>{p}</option>
                                        ))}
                                    </select>
                                    <button
                                        className={styles.removeBtn}
                                        onClick={() => removeFile(f.id)}
                                        disabled={busy}
                                        title="Remove"
                                    >
                                        ×
                                    </button>
                                </div>
                            )
                        })}
                    </div>

                    {/* ── Feedback + submit ── */}
                    <div className={styles.submitRow}>
                        {phase === 'ok' && (
                            <div className={`${styles.toast} ${styles.toastOk}`}>
                                <span>{submitted} job{submitted !== 1 ? 's' : ''} queued successfully.</span>
                                <button className={styles.toastClose} onClick={() => setPhase('idle')}>×</button>
                            </div>
                        )}
                        {phase === 'err' && (
                            <div className={`${styles.toast} ${styles.toastErr}`}>
                                <span>{errMsg}</span>
                                <button className={styles.toastClose} onClick={() => setPhase('idle')}>×</button>
                            </div>
                        )}
                        <button
                            className={styles.submitBtn}
                            onClick={handleSubmit}
                            disabled={busy}
                        >
                            {phase === 'uploading'
                                ? 'Uploading…'
                                : phase === 'busy'
                                    ? 'Submitting…'
                                    : `Submit ${files.length} Job${files.length !== 1 ? 's' : ''}`}
                        </button>
                    </div>
                </>
            )}
        </div>
    )
}
