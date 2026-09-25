import { useState } from 'react'
import { OPERATION_LABEL, opArrow } from '../api'
import StatusBadge from './StatusBadge'
import styles from './JobTable.module.css'

function ProgressBar({ value, status }) {
    if (status === 'pending' || status === 'assigned') return <span className="muted">—</span>
    const color = status === 'failed' ? 'var(--red)' : status === 'completed' ? 'var(--green)' : 'var(--accent)'
    return (
        <div className="bar">
            <div className="track"><div className="fill" style={{ width: `${value}%`, background: color }} /></div>
            <span className={styles.pct}>{value}%</span>
        </div>
    )
}

const FILTERS = ['all', 'pending', 'assigned', 'running']
const FILTER_LABEL = { all: 'todas', pending: 'pendientes', assigned: 'asignadas', running: 'en ejecución' }

function shortFile(p) { return p ? (p.split(/[\\/]/).pop() || p) : '—' }

export default function JobTable({ jobs }) {
    const [filter, setFilter] = useState('all')
    const [search, setSearch] = useState('')

    const q = search.toLowerCase()
    const visible = jobs
        .filter(j => filter === 'all' || j.status === filter)
        .filter(j => !q || j.id.includes(q) || (j.operation || '').includes(q) || (j.worker_id || '').toLowerCase().includes(q) || (j.file_path || '').toLowerCase().includes(q))
        .slice(0, 200)

    return (
        <div className={styles.wrap}>
            <div className={styles.toolbar}>
                <div className="filters">
                    {FILTERS.map(f => (
                        <button key={f} className={`filter ${filter === f ? 'active' : ''}`} onClick={() => setFilter(f)}>{FILTER_LABEL[f]}</button>
                    ))}
                </div>
                <input className={`input ${styles.search}`} placeholder="Buscar por id, archivo, operación o worker…" value={search} onChange={e => setSearch(e.target.value)} />
            </div>

            <div className="tableWrap">
                <table className="table">
                    <thead>
                        <tr>
                            <th>Sub-tarea</th><th>Archivo</th><th>Operación</th><th>Pool</th><th>Estado</th>
                            <th>Progreso</th><th>Worker</th><th>Prioridad</th><th>Creada</th>
                        </tr>
                    </thead>
                    <tbody>
                        {visible.length === 0 && (
                            <tr><td colSpan={9} className="empty">Ninguna sub-tarea con ese filtro.</td></tr>
                        )}
                        {visible.map(job => (
                            <tr key={job.id}>
                                <td className="mono muted">{job.id.slice(0, 8)}</td>
                                <td className={styles.file} title={job.file_path}>{shortFile(job.file_path)}</td>
                                <td title={OPERATION_LABEL[job.operation] || job.operation}><span className="mono">{opArrow(job)}</span> <span className="muted">{OPERATION_LABEL[job.operation]}</span></td>
                                <td>{job.pool ? <span className={`chip pool pool-${job.pool}`}>{job.pool}</span> : <span className="muted">—</span>}</td>
                                <td><StatusBadge status={job.status} kind="job" /></td>
                                <td><ProgressBar value={job.progress ?? 0} status={job.status} /></td>
                                <td className="muted">{job.worker_id || '—'}{job.assignment === 'ayuda' && <span className="chip chip-yellow" style={{ marginLeft: 6 }} title="Un nodo de otro pool la tomó porque estaba libre">ayuda</span>}</td>
                                <td className="muted num">{job.priority}</td>
                                <td className="muted num">{new Date(job.created_at).toLocaleTimeString()}</td>
                            </tr>
                        ))}
                    </tbody>
                </table>
            </div>
            <div className={styles.count}>{visible.length} de {jobs.length} sub-tareas</div>
        </div>
    )
}
