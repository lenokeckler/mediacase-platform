import { useState, useEffect, useCallback } from 'react'
import { api } from '../api'
import styles from './JobHistory.module.css'

const STATUS_COLORS = {
    pending:   { bg: '#1e293b', text: '#94a3b8' },
    assigned:  { bg: '#1e3a5f', text: '#60a5fa' },
    running:   { bg: '#1c3829', text: '#4ade80' },
    completed: { bg: '#14532d', text: '#86efac' },
    failed:    { bg: '#450a0a', text: '#fca5a5' },
}

const FILTERS = ['all', 'completed', 'failed', 'running', 'pending', 'assigned']

function StatusBadge({ status }) {
    const c = STATUS_COLORS[status] || { bg: '#1a1d2e', text: '#94a3b8' }
    return (
        <span className={styles.badge} style={{ background: c.bg, color: c.text }}>
            {status}
        </span>
    )
}

function calcDuration(startedAt, completedAt) {
    if (!startedAt || !completedAt) return '—'
    const ms = new Date(completedAt) - new Date(startedAt)
    if (isNaN(ms) || ms < 0) return '—'
    if (ms < 1000) return `${ms}ms`
    if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
    return `${Math.floor(ms / 60000)}m ${Math.floor((ms % 60000) / 1000)}s`
}

function fmtDate(dt) {
    if (!dt) return '—'
    const d = new Date(dt)
    return isNaN(d) ? '—' : d.toLocaleString()
}

function fmtTime(dt) {
    if (!dt) return '—'
    const d = new Date(dt)
    return isNaN(d) ? '—' : d.toLocaleTimeString()
}

function shortFile(filePath) {
    if (!filePath) return '—'
    return filePath.split(/[\\/]/).pop() || filePath
}

export default function JobHistory() {
    const [jobs, setJobs] = useState([])
    const [loading, setLoading] = useState(false)
    const [error, setError] = useState('')
    const [filter, setFilter] = useState('all')
    const [search, setSearch] = useState('')
    const [expandedId, setExpandedId] = useState(null)

    const load = useCallback(async () => {
        setLoading(true)
        setError('')
        try {
            const data = await api.listJobs()
            const sorted = (Array.isArray(data) ? data : [])
                .sort((a, b) => new Date(b.created_at) - new Date(a.created_at))
            setJobs(sorted)
        } catch (err) {
            setError(err.message)
        } finally {
            setLoading(false)
        }
    }, [])

    useEffect(() => { load() }, [load])

    const counts = FILTERS.reduce((acc, f) => {
        acc[f] = f === 'all' ? jobs.length : jobs.filter(j => j.status === f).length
        return acc
    }, {})

    const visible = jobs
        .filter(j => filter === 'all' || j.status === filter)
        .filter(j => {
            if (!search) return true
            const q = search.toLowerCase()
            return (
                j.id.toLowerCase().includes(q) ||
                (j.file_path || '').toLowerCase().includes(q) ||
                (j.operation || '').toLowerCase().includes(q) ||
                (j.worker_id || '').toLowerCase().includes(q)
            )
        })

    return (
        <div className={styles.wrap}>
            <div className={styles.toolbar}>
                <div className={styles.filters}>
                    {FILTERS.map(f => (
                        <button
                            key={f}
                            className={`${styles.filterBtn} ${filter === f ? styles.active : ''}`}
                            onClick={() => setFilter(f)}
                        >
                            {f}
                            {counts[f] > 0 && (
                                <span className={styles.pill}>{counts[f]}</span>
                            )}
                        </button>
                    ))}
                </div>
                <input
                    className={styles.search}
                    placeholder="Search by ID, file, operation, worker…"
                    value={search}
                    onChange={e => setSearch(e.target.value)}
                />
                <button className={styles.refreshBtn} onClick={load} disabled={loading}>
                    {loading ? '…' : '↻ Refresh'}
                </button>
            </div>

            {error && <div className={styles.errorBanner}>{error}</div>}

            <div className={styles.tableWrap}>
                <table className={styles.table}>
                    <thead>
                        <tr>
                            <th>Job ID</th>
                            <th>File</th>
                            <th>Operation</th>
                            <th>Status</th>
                            <th>Progress</th>
                            <th>Worker</th>
                            <th>Priority</th>
                            <th>Created</th>
                            <th>Duration</th>
                            <th>Result / Error</th>
                        </tr>
                    </thead>
                    <tbody>
                        {visible.length === 0 && (
                            <tr>
                                <td colSpan={10} className={styles.empty}>
                                    {loading ? 'Loading…' : 'No jobs match the current filter.'}
                                </td>
                            </tr>
                        )}
                        {visible.map(job => (
                            <>
                                <tr
                                    key={job.id}
                                    className={`${styles.row} ${expandedId === job.id ? styles.rowExpanded : ''}`}
                                    onClick={() => setExpandedId(expandedId === job.id ? null : job.id)}
                                    title="Click to expand details"
                                >
                                    <td className={styles.jobId}>{job.id.slice(0, 8)}…</td>
                                    <td className={styles.fileName} title={job.file_path}>
                                        {shortFile(job.file_path)}
                                    </td>
                                    <td>
                                        <span className={styles.op}>{job.operation}</span>
                                    </td>
                                    <td><StatusBadge status={job.status} /></td>
                                    <td className={styles.muted}>
                                        {job.status === 'pending' || job.status === 'assigned'
                                            ? '—'
                                            : `${job.progress ?? 0}%`}
                                    </td>
                                    <td className={styles.muted}>{job.worker_id || '—'}</td>
                                    <td className={styles.muted}>{job.priority}</td>
                                    <td className={styles.muted}>{fmtTime(job.created_at)}</td>
                                    <td className={styles.muted}>
                                        {calcDuration(job.started_at, job.completed_at)}
                                    </td>
                                    <td>
                                        {job.result_url ? (
                                            <a
                                                className={styles.downloadLink}
                                                href={job.result_url}
                                                target="_blank"
                                                rel="noopener noreferrer"
                                                onClick={e => e.stopPropagation()}
                                            >
                                                Download
                                            </a>
                                        ) : job.error_msg ? (
                                            <span className={styles.errorText} title={job.error_msg}>
                                                {job.error_msg.slice(0, 35)}{job.error_msg.length > 35 ? '…' : ''}
                                            </span>
                                        ) : '—'}
                                    </td>
                                </tr>
                                {expandedId === job.id && (
                                    <tr key={`${job.id}-detail`} className={styles.detailRow}>
                                        <td colSpan={10}>
                                            <div className={styles.detail}>
                                                <div className={styles.detailGrid}>
                                                    <div className={styles.detailItem}>
                                                        <span className={styles.detailLabel}>Full Job ID</span>
                                                        <code className={styles.detailValue}>{job.id}</code>
                                                    </div>
                                                    <div className={styles.detailItem}>
                                                        <span className={styles.detailLabel}>Full File Path</span>
                                                        <code className={styles.detailValue}>{job.file_path || '—'}</code>
                                                    </div>
                                                    <div className={styles.detailItem}>
                                                        <span className={styles.detailLabel}>Started At</span>
                                                        <span className={styles.detailValue}>{fmtDate(job.started_at)}</span>
                                                    </div>
                                                    <div className={styles.detailItem}>
                                                        <span className={styles.detailLabel}>Completed At</span>
                                                        <span className={styles.detailValue}>{fmtDate(job.completed_at)}</span>
                                                    </div>
                                                    <div className={styles.detailItem}>
                                                        <span className={styles.detailLabel}>Retries</span>
                                                        <span className={styles.detailValue}>{job.retries} / {job.max_retries}</span>
                                                    </div>
                                                    {job.error_msg && (
                                                        <div className={`${styles.detailItem} ${styles.detailFull}`}>
                                                            <span className={styles.detailLabel}>Error Message</span>
                                                            <span className={`${styles.detailValue} ${styles.errorFull}`}>{job.error_msg}</span>
                                                        </div>
                                                    )}
                                                    {job.result_url && (
                                                        <div className={`${styles.detailItem} ${styles.detailFull}`}>
                                                            <span className={styles.detailLabel}>Result URL</span>
                                                            <a
                                                                className={styles.downloadLink}
                                                                href={job.result_url}
                                                                target="_blank"
                                                                rel="noopener noreferrer"
                                                            >
                                                                {job.result_url}
                                                            </a>
                                                        </div>
                                                    )}
                                                </div>
                                            </div>
                                        </td>
                                    </tr>
                                )}
                            </>
                        ))}
                    </tbody>
                </table>
            </div>
            <div className={styles.count}>{visible.length} of {jobs.length} jobs</div>
        </div>
    )
}
