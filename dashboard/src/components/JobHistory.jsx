import { useState, useEffect, useCallback } from 'react'
import { api, OPERATION_LABEL } from '../api'
import StatusBadge from './StatusBadge'
import styles from './JobHistory.module.css'

const FILTERS = ['all', 'completed', 'failed', 'cancelled', 'running', 'pending', 'assigned']
const FILTER_LABEL = { all: 'todas', pending: 'pendientes', assigned: 'asignadas', running: 'en ejecución', completed: 'completadas', failed: 'fallidas', cancelled: 'canceladas' }

function calcDuration(startedAt, completedAt) {
    if (!startedAt || !completedAt) return '—'
    const ms = new Date(completedAt) - new Date(startedAt)
    if (isNaN(ms) || ms < 0) return '—'
    if (ms < 1000) return `${ms} ms`
    if (ms < 60000) return `${(ms / 1000).toFixed(1)} s`
    return `${Math.floor(ms / 60000)} min ${Math.floor((ms % 60000) / 1000)} s`
}
function fmtDate(dt) { if (!dt) return '—'; const d = new Date(dt); return isNaN(d) ? '—' : d.toLocaleString() }
function fmtTime(dt) { if (!dt) return '—'; const d = new Date(dt); return isNaN(d) ? '—' : d.toLocaleTimeString() }
function shortFile(p) { return p ? (p.split(/[\\/]/).pop() || p) : '—' }

// Todas las sub-tareas (también las terminadas) con filtro, búsqueda y detalle desplegable.
export default function JobHistory() {
    const [jobs, setJobs] = useState([])
    const [loading, setLoading] = useState(false)
    const [error, setError] = useState('')
    const [filter, setFilter] = useState('all')
    const [search, setSearch] = useState('')
    const [expandedId, setExpandedId] = useState(null)

    const load = useCallback(async () => {
        setLoading(true); setError('')
        try {
            const data = await api.listJobs()
            setJobs((Array.isArray(data) ? data : []).sort((a, b) => new Date(b.created_at) - new Date(a.created_at)))
        } catch (err) { setError(err.message) } finally { setLoading(false) }
    }, [])
    useEffect(() => { load() }, [load])

    const counts = FILTERS.reduce((acc, f) => ({ ...acc, [f]: f === 'all' ? jobs.length : jobs.filter(j => j.status === f).length }), {})
    const q = search.toLowerCase()
    const visible = jobs
        .filter(j => filter === 'all' || j.status === filter)
        .filter(j => !q || j.id.toLowerCase().includes(q) || (j.file_path || '').toLowerCase().includes(q) || (j.operation || '').toLowerCase().includes(q) || (j.worker_id || '').toLowerCase().includes(q))

    return (
        <div className={styles.wrap}>
            <div className={styles.toolbar}>
                <div className="filters">
                    {FILTERS.map(f => (
                        <button key={f} className={`filter ${filter === f ? 'active' : ''}`} onClick={() => setFilter(f)}>
                            {FILTER_LABEL[f]}{counts[f] > 0 && <span className="pill">{counts[f]}</span>}
                        </button>
                    ))}
                </div>
                <input className={`input ${styles.search}`} placeholder="Buscar por id, archivo, operación o worker…" value={search} onChange={e => setSearch(e.target.value)} />
                <button className="btn" onClick={load} disabled={loading}>{loading ? '…' : '↻ Actualizar'}</button>
            </div>

            {error && <div className="alert alert-red">{error}</div>}

            <div className="tableWrap">
                <table className="table">
                    <thead>
                        <tr>
                            <th>Sub-tarea</th><th>Archivo</th><th>Operación</th><th>Estado</th><th>Progreso</th>
                            <th>Worker</th><th>Prioridad</th><th>Creada</th><th>Duración</th><th>Resultado / error</th>
                        </tr>
                    </thead>
                    <tbody>
                        {visible.length === 0 && (
                            <tr><td colSpan={10} className="empty">{loading ? 'Cargando…' : 'Ninguna sub-tarea con ese filtro.'}</td></tr>
                        )}
                        {visible.map(job => (
                            <JobRows key={job.id} job={job} expanded={expandedId === job.id}
                                onToggle={() => setExpandedId(expandedId === job.id ? null : job.id)} />
                        ))}
                    </tbody>
                </table>
            </div>
            <div className={styles.count}>{visible.length} de {jobs.length} sub-tareas</div>
        </div>
    )
}

function JobRows({ job, expanded, onToggle }) {
    return (
        <>
            <tr className={`${styles.row} ${expanded ? styles.rowExpanded : ''}`} onClick={onToggle} title="Clic para ver el detalle">
                <td className="mono muted">{job.id.slice(0, 8)}</td>
                <td className={styles.file} title={job.file_path}>{shortFile(job.file_path)}</td>
                <td>{OPERATION_LABEL[job.operation] || job.operation}</td>
                <td><StatusBadge status={job.status} kind="job" /></td>
                <td className="muted num">{job.status === 'pending' || job.status === 'assigned' ? '—' : `${job.progress ?? 0}%`}</td>
                <td className="muted">{job.worker_id || '—'}</td>
                <td className="muted num">{job.priority}</td>
                <td className="muted num">{fmtTime(job.created_at)}</td>
                <td className="muted num">{calcDuration(job.started_at, job.completed_at)}</td>
                <td>
                    {job.result_url ? (
                        <a className={styles.link} href={job.result_url} target="_blank" rel="noopener noreferrer" onClick={e => e.stopPropagation()}>Descargar</a>
                    ) : job.error_msg ? (
                        <span className={styles.err} title={job.error_msg}>{job.error_msg.slice(0, 40)}{job.error_msg.length > 40 ? '…' : ''}</span>
                    ) : <span className="muted">—</span>}
                </td>
            </tr>
            {expanded && (
                <tr className={styles.detailRow}>
                    <td colSpan={10}>
                        <div className={styles.detailGrid}>
                            <Item label="Id completo"><code className="mono">{job.id}</code></Item>
                            <Item label="Archivo"><code className="mono">{job.file_path || '—'}</code></Item>
                            <Item label="Caso"><code className="mono">{job.case_id || '—'}</code></Item>
                            <Item label="Inicio">{fmtDate(job.started_at)}</Item>
                            <Item label="Fin">{fmtDate(job.completed_at)}</Item>
                            <Item label="Reintentos">{job.retries} / {job.max_retries}</Item>
                            {job.error_msg && <Item label="Error" full><span className={styles.err}>{job.error_msg}</span></Item>}
                            {job.result_url && <Item label="Resultado" full><a className={styles.link} href={job.result_url} target="_blank" rel="noopener noreferrer">{job.result_url}</a></Item>}
                        </div>
                    </td>
                </tr>
            )}
        </>
    )
}

function Item({ label, children, full }) {
    return (
        <div className={`${styles.item} ${full ? styles.itemFull : ''}`}>
            <span className={styles.itemLabel}>{label}</span>
            <span className={styles.itemValue}>{children}</span>
        </div>
    )
}
