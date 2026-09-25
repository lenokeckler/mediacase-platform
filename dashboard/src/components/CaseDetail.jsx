import { useState } from 'react'
import { fmtSeconds, fmtTime, isTerminal, OPERATION_LABEL, opArrow, TYPE_LABEL } from '../api'
import StatusBadge from './StatusBadge'
import EnrichmentChips from './EnrichmentChips'
import styles from './CaseDetail.module.css'

function Progress({ job }) {
    if (job.status === 'pending' || job.status === 'assigned' || job.status === 'cancelled') {
        return <span className={styles.muted}>—</span>
    }
    const color = job.status === 'failed' ? 'var(--red)' : job.status === 'completed' ? 'var(--green)' : 'var(--accent)'
    return (
        <div className={styles.progressWrap}>
            <div className={styles.progressTrack}>
                <div className={styles.progressFill} style={{ width: `${job.progress}%`, background: color }} />
            </div>
            <span className={styles.muted}>{job.progress}%</span>
        </div>
    )
}

function jobDuration(j) {
    if (!j.started_at) return null
    const end = j.completed_at ? new Date(j.completed_at) : new Date()
    return (end - new Date(j.started_at)) / 1000
}

export default function CaseDetail({ c, report, onCancel, onClose }) {
    const [confirming, setConfirming] = useState(false)
    const jobs = c.jobs || []
    const resolved = jobs.filter(j => ['completed', 'failed', 'cancelled'].includes(j.status)).length
    const terminal = isTerminal(c.status)
    const caseDur = c.started_at ? ((c.completed_at ? new Date(c.completed_at) : new Date()) - new Date(c.started_at)) / 1000 : null

    async function cancel() {
        if (!confirming) { setConfirming(true); return }
        setConfirming(false)
        await onCancel(c.id)
    }

    const reportClass = c.status === 'partially_completed' ? styles.reportPartial
        : (c.status === 'cancelled' || c.status === 'failed') ? styles.reportCancelled : ''

    return (
        <div className={styles.panel}>
            <div className={styles.header}>
                <div className={styles.titleGroup}>
                    <div className={styles.title}>
                        {c.name || '(sin nombre)'} <StatusBadge status={c.status} />
                    </div>
                    <span className={styles.id}>{c.id}</span>
                    <div className={styles.meta}>
                        <span>prioridad <strong>{c.priority}</strong></span>
                        <span>sub-tareas <strong>{resolved}/{c.total_jobs}</strong> resueltas</span>
                        <span>creado <strong>{fmtTime(c.created_at)}</strong></span>
                        <span>inicio <strong>{fmtTime(c.started_at)}</strong></span>
                        <span>cierre <strong>{fmtTime(c.completed_at)}</strong></span>
                        <span>duración <strong>{caseDur == null ? '—' : fmtSeconds(caseDur)}</strong></span>
                    </div>
                </div>
                <div className={styles.actions}>
                    {!terminal && (
                        <button className={`${styles.btn} ${styles.danger}`} onClick={cancel} onBlur={() => setConfirming(false)}>
                            {confirming ? '¿Seguro? Clic otra vez para cancelar' : 'Cancelar caso'}
                        </button>
                    )}
                    {terminal && (
                        <a className={styles.btn} href={`/api/cases/${c.id}/report`} target="_blank" rel="noreferrer">Reporte (JSON)</a>
                    )}
                    <button className={styles.btn} onClick={onClose}>Cerrar</button>
                </div>
            </div>

            {terminal && report && (
                <div className={`${styles.report} ${reportClass}`}>
                    <span className={styles.reportTitle}>Reporte consolidado</span>
                    <div className={styles.summary}>{report.summary}</div>
                    <div className={styles.groups}>
                        {report.by_type_and_operation.map(g => (
                            <span key={`${g.file_type}/${g.operation}`} className={styles.group}>
                                {(TYPE_LABEL[g.file_type] || g.file_type).toLowerCase()} · {OPERATION_LABEL[g.operation] || g.operation}{g.target && g.operation !== 'metadata' ? ` → ${g.target.toUpperCase()}` : ''}: {g.completed} ok
                                {g.failed ? `, ${g.failed} fallida${g.failed === 1 ? '' : 's'}` : ''}
                                {g.cancelled ? `, ${g.cancelled} cancelada${g.cancelled === 1 ? '' : 's'}` : ''}
                            </span>
                        ))}
                        <span className={styles.group}>duración total: {fmtSeconds(report.duration_seconds)}</span>
                    </div>
                </div>
            )}
            {terminal && !report && <div className={styles.muted}>Generando reporte…</div>}

            <div className={styles.tableWrap}>
                <table className={styles.table}>
                    <thead>
                        <tr>
                            <th>Archivo</th><th>Tipo</th><th>Operación</th><th>Pool</th><th>Estado</th>
                            <th>Progreso</th><th>Worker</th><th>Inicio</th><th>Duración</th><th>Resultado</th>
                        </tr>
                    </thead>
                    <tbody>
                        {jobs.map(j => {
                            const d = jobDuration(j)
                            return (
                                <tr key={j.id}>
                                    <td className={styles.file}>{j.file_path}</td>
                                    <td>{(TYPE_LABEL[j.file_type] || j.file_type).toLowerCase()}</td>
                                    <td title={OPERATION_LABEL[j.operation] || j.operation}>
                                        <span className="mono">{opArrow(j)}</span> <span className="muted">{OPERATION_LABEL[j.operation]}</span>
                                        <EnrichmentChips job={j} />
                                        {j.routing_note && (
                                            <span className="chip chip-yellow" style={{ marginLeft: 6 }} title={j.routing_note}>⚠ aviso</span>
                                        )}
                                    </td>
                                    <td><span className={styles.pool}>{j.pool}</span></td>
                                    <td><StatusBadge status={j.status} kind="job" /></td>
                                    <td><Progress job={j} /></td>
                                    <td>{j.worker_id || <span className={styles.muted}>—</span>}{j.assignment === 'ayuda' && <span className="chip chip-yellow" style={{ marginLeft: 6 }} title="Un nodo de otro pool la tomó porque estaba libre">ayuda</span>}</td>
                                    <td className={styles.muted}>{fmtTime(j.started_at)}</td>
                                    <td className={styles.muted}>{d == null ? '—' : fmtSeconds(d)}</td>
                                    <td>
                                        {j.status === 'completed' && j.result_url && (
                                            <a className={styles.link} href={j.result_url} target="_blank" rel="noreferrer">Descargar</a>
                                        )}
                                        {j.status === 'failed' && <span className={styles.err}>{j.error_msg}</span>}
                                    </td>
                                </tr>
                            )
                        })}
                        {jobs.length === 0 && <tr><td colSpan={10} className={styles.empty}>Sin sub-tareas.</td></tr>}
                    </tbody>
                </table>
            </div>
        </div>
    )
}
