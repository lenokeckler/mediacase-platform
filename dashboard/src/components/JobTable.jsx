import { useState } from 'react'
import styles from './JobTable.module.css'

const STATUS_COLORS = {
    pending: { bg: '#1e293b', text: '#94a3b8' },
    assigned: { bg: '#1e3a5f', text: '#60a5fa' },
    running: { bg: '#1c3829', text: '#4ade80' },
    completed: { bg: '#14532d', text: '#86efac' },
    failed: { bg: '#450a0a', text: '#fca5a5' },
}

function StatusBadge({ status }) {
    const c = STATUS_COLORS[status] || { bg: '#1a1d2e', text: '#94a3b8' }
    return (
        <span className={styles.badge} style={{ background: c.bg, color: c.text }}>
            {status}
        </span>
    )
}

function ProgressBar({ value, status }) {
    if (status === 'pending' || status === 'assigned') return <span className={styles.muted}>—</span>
    const color = status === 'failed' ? 'var(--red)' : status === 'completed' ? 'var(--green)' : 'var(--accent)'
    return (
        <div className={styles.progressWrap}>
            <div className={styles.progressTrack}>
                <div className={styles.progressFill} style={{ width: `${value}%`, background: color }} />
            </div>
            <span className={styles.progressLabel}>{value}%</span>
        </div>
    )
}

const FILTERS = ['all', 'pending', 'running', 'completed', 'failed']

export default function JobTable({ jobs }) {
    const [filter, setFilter] = useState('all')
    const [search, setSearch] = useState('')

    const visible = jobs
        .filter(j => filter === 'all' || j.status === filter)
        .filter(j => !search || j.id.includes(search) || j.operation.includes(search) || (j.worker_id || '').includes(search))
        .slice(0, 200)

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
                        </button>
                    ))}
                </div>
                <input
                    className={styles.search}
                    placeholder="Search job ID, operation, worker…"
                    value={search}
                    onChange={e => setSearch(e.target.value)}
                />
            </div>

            <div className={styles.tableWrap}>
                <table className={styles.table}>
                    <thead>
                        <tr>
                            <th>Job ID</th>
                            <th>Operation</th>
                            <th>Status</th>
                            <th>Progress</th>
                            <th>Worker</th>
                            <th>Priority</th>
                            <th>Created</th>
                        </tr>
                    </thead>
                    <tbody>
                        {visible.length === 0 && (
                            <tr>
                                <td colSpan={7} className={styles.empty}>No jobs match the current filter.</td>
                            </tr>
                        )}
                        {visible.map(job => (
                            <tr key={job.id} className={styles.row}>
                                <td className={styles.jobId}>{job.id.slice(0, 8)}…</td>
                                <td>
                                    <span className={styles.op}>{job.operation}</span>
                                </td>
                                <td><StatusBadge status={job.status} /></td>
                                <td><ProgressBar value={job.progress ?? 0} status={job.status} /></td>
                                <td className={styles.muted}>{job.worker_id || '—'}</td>
                                <td className={styles.muted}>{job.priority}</td>
                                <td className={styles.muted}>{new Date(job.created_at).toLocaleTimeString()}</td>
                            </tr>
                        ))}
                    </tbody>
                </table>
            </div>
            <div className={styles.count}>{visible.length} of {jobs.length} jobs</div>
        </div>
    )
}