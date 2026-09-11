import { useState } from 'react'
import { api } from '../api'
import styles from './BatchPanel.module.css'

const DEFAULT_SERVER_PATH = '/app/dataset/files'
const DEFAULT_PRIORITY = 5

const VIDEO_EXTS = new Set(['mp4', 'mkv', 'avi', 'mov', 'webm'])
const MEDIA_EXTS = new Set(['mp4', 'mkv', 'avi', 'mov', 'webm', 'mp3', 'wav', 'aac', 'flac', 'ogg'])

const VIDEO_OPS = [
    { value: 'convert',       label: 'Convert to MP4',      desc: 'Re-encode as H.264 + AAC' },
    { value: 'extract_audio', label: 'Extract Audio (MP3)',  desc: 'Strip video stream, export MP3' },
    { value: 'thumbnail',     label: 'Generate Thumbnail',   desc: 'Capture a JPEG frame at 5 s' },
]

const AUDIO_OPS = [
    { value: 'extract_audio', label: 'Re-encode to MP3',    desc: 'Normalize audio to 192k MP3' },
    { value: 'convert_audio', label: 'Convert to WAV',      desc: 'Export PCM 16-bit 44.1 kHz' },
    { value: 'thumbnail',     label: 'Generate Waveform',   desc: 'Render a PNG waveform image' },
]

function getType(name) {
    const ext = name.toLowerCase().split('.').pop()
    return VIDEO_EXTS.has(ext) ? 'video' : 'audio'
}

function fmtSize(bytes) {
    if (!bytes) return '—'
    if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KB`
    if (bytes < 1073741824) return `${(bytes / 1048576).toFixed(1)} MB`
    return `${(bytes / 1073741824).toFixed(2)} GB`
}

// ── Operation picker ──────────────────────────────────────────────────────────

function OpPicker({ fileType, ops, value, onChange }) {
    return (
        <div className={styles.opPicker}>
            <div className={styles.opPickerTitle}>
                {fileType === 'video' ? '🎬 Video' : '🎵 Audio'} files — choose operation
            </div>
            <div className={styles.opCards}>
                {ops.map(op => (
                    <label
                        key={op.value}
                        className={`${styles.opCard} ${value === op.value ? styles.opCardActive : ''}`}
                    >
                        <input
                            type="radio"
                            name={`op-${fileType}`}
                            value={op.value}
                            checked={value === op.value}
                            onChange={() => onChange(op.value)}
                            className={styles.opRadio}
                        />
                        <span className={styles.opLabel}>{op.label}</span>
                        <span className={styles.opDesc}>{op.desc}</span>
                    </label>
                ))}
            </div>
        </div>
    )
}

// ── File list ─────────────────────────────────────────────────────────────────

function FileList({ files }) {
    return (
        <div className={styles.fileList}>
            {files.map((f, i) => (
                <div key={i} className={styles.fileRow}>
                    <span
                        className={styles.typeBadge}
                        style={f.type === 'video'
                            ? { background: '#1e3a5f', color: '#60a5fa' }
                            : { background: '#1c2e1c', color: '#4ade80' }}
                    >
                        {f.type}
                    </span>
                    <span className={styles.fileName} title={f.name}>{f.name}</span>
                    <span className={styles.fileSize}>{fmtSize(f.size)}</span>
                </div>
            ))}
        </div>
    )
}

// ── Main panel ────────────────────────────────────────────────────────────────

export default function BatchPanel() {
    const [files, setFiles] = useState([])
    const [folderName, setFolderName] = useState('')
    const [serverPath, setServerPath] = useState(DEFAULT_SERVER_PATH)
    const [showPathEdit, setShowPathEdit] = useState(false)
    const [videoOp, setVideoOp] = useState('convert')
    const [audioOp, setAudioOp] = useState('extract_audio')
    const [phase, setPhase] = useState('idle')   // idle | loading | busy | ok | err
    const [submitted, setSubmitted] = useState(0)
    const [errMsg, setErrMsg] = useState('')

    async function handleLoadFromServer() {
        setPhase('loading')
        setErrMsg('')
        try {
            const data = await api.listFiles(serverPath)
            if (!data.files || data.files.length === 0) {
                throw new Error('No media files found in that directory.')
            }
            // Filter by known media extensions
            const media = data.files.filter(f => {
                const ext = f.filename.toLowerCase().split('.').pop()
                return MEDIA_EXTS.has(ext)
            })
            
            setFiles(media.map(f => ({ name: f.filename, size: f.size_bytes, type: f.type })))
            setFolderName(data.folder_path)
            setPhase('idle')
            setSubmitted(0)
        } catch (err) {
            setErrMsg(err.message)
            setPhase('err')
        }
    }

    async function handleSubmit() {
        if (!files.length) return
        setPhase('busy')
        setErrMsg('')
        try {
            const jobs = files.map(f => ({
                file_path: `${serverPath.replace(/\/$/, '')}/${f.name}`,
                operation: f.type === 'video' ? videoOp : audioOp,
                priority: DEFAULT_PRIORITY
            }))

            const result = await api.submitBatch(jobs)
            setSubmitted(Array.isArray(result) ? result.length : jobs.length)
            setPhase('ok')
            setFiles([])
        } catch (err) {
            setErrMsg(err.message)
            setPhase('err')
        }
    }

    const videoCount = files.filter(f => f.type === 'video').length
    const audioCount = files.filter(f => f.type === 'audio').length

    return (
        <div className={styles.panel}>
            <div className={styles.panelHeader}>
                <h3 className={styles.title}>Automatic / Batch Processing</h3>
                <p className={styles.desc}>Select a local folder to load its media files and queue them all at once.</p>
            </div>

            {/* ── Folder picker ── */}
            <div className={styles.pickerSection}>
                <button
                    className={styles.browseBtn}
                    onClick={handleLoadFromServer}
                    disabled={phase === 'loading' || phase === 'busy'}
                >
                    {phase === 'loading' ? 'Loading...' : '📥 Load Dataset from Server'}
                </button>

                {folderName && (
                    <span className={styles.folderName}>
                        {folderName}
                        <span className={styles.folderCount}>
                            {files.length} media file{files.length !== 1 ? 's' : ''} found
                        </span>
                    </span>
                )}
            </div>

            {/* ── Server path (advanced, collapsed by default) ── */}
            <div className={styles.advancedRow}>
                <button
                    className={styles.advancedToggle}
                    onClick={() => setShowPathEdit(v => !v)}
                >
                    {showPathEdit ? '▾' : '▸'} Server path prefix
                </button>
                {showPathEdit && (
                    <input
                        className={styles.pathInput}
                        type="text"
                        value={serverPath}
                        onChange={e => setServerPath(e.target.value)}
                        placeholder="/app/dataset/files"
                    />
                )}
                {!showPathEdit && (
                    <span className={styles.pathPreview}>{serverPath}</span>
                )}
            </div>

            {/* ── Loaded content ── */}
            {files.length > 0 && (
                <>
                    {/* Summary */}
                    <div className={styles.summary}>
                        <span className={styles.summaryTotal}>{files.length} files loaded</span>
                        {videoCount > 0 && (
                            <span className={styles.summaryChip} style={{ color: '#60a5fa' }}>
                                🎬 {videoCount} video
                            </span>
                        )}
                        {audioCount > 0 && (
                            <span className={styles.summaryChip} style={{ color: '#4ade80' }}>
                                🎵 {audioCount} audio
                            </span>
                        )}
                    </div>

                    {/* Operation pickers */}
                    <div className={styles.opSection}>
                        {videoCount > 0 && (
                            <OpPicker
                                fileType="video"
                                ops={VIDEO_OPS}
                                value={videoOp}
                                onChange={setVideoOp}
                            />
                        )}
                        {audioCount > 0 && (
                            <OpPicker
                                fileType="audio"
                                ops={AUDIO_OPS}
                                value={audioOp}
                                onChange={setAudioOp}
                            />
                        )}
                    </div>

                    {/* File list */}
                    <div className={styles.fileSection}>
                        <span className={styles.label}>Files</span>
                        <FileList files={files} />
                    </div>

                    {/* Feedback + submit */}
                    <div className={styles.submitRow}>
                        {phase === 'ok' && (
                            <div className={`${styles.toast} ${styles.toastOk}`}>
                                <span>{submitted} jobs queued successfully.</span>
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
                            disabled={phase === 'uploading' || phase === 'busy'}
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
