import { useState } from 'react'
import { fmtSeconds, fmtTime, isTerminal } from '../api'
import StatusBadge from './StatusBadge'
import styles from './CasesPanel.module.css'

const FILTERS = [
    ['', 'todos'], ['queued', 'en cola'], ['processing', 'procesando'], ['retrying', 'reintentando'],
    ['completed', 'completados'], ['partially_completed', 'parciales'], ['failed', 'fallidos'], ['cancelled', 'cancelados'],
]

function durationOf(c) {
    if (!c.started_at) return null
    const end = c.completed_at ? new Date(c.completed_at) : new Date()
    return (end - new Date(c.started_at)) / 1000
}

// Lista de casos con su estado agregado. Los conteos de sub-tareas vienen del detalle
// (GET /cases/{id}) solo para el caso abierto; en la lista se muestra total_jobs.
export default function CasesPanel({ cases, error, selectedId, onOpen, onNew }) {
    const [filter, setFilter] = useState('')

    const visible = cases.filter(c => !filter || c.status === filter)

    return (
        <div className={styles.wrap}>
            <div className={styles.toolbar}>
                <div className={styles.filters}>
                    {FILTERS.map(([value, label]) => (
                        <button
                            key={value}
                            className={`${styles.filterBtn} ${filter === value ? styles.filterActive : ''}`}
                            onClick={() => setFilter(value)}
                        >
                            {label}
                        </button>
                    ))}
                </div>
                <span className={styles.spacer} />
                <button className={styles.newBtn} onClick={onNew}>+ Nuevo caso</button>
            </div>

            {error && <div className={styles.error}>No se pudo consultar los casos: {error}</div>}

            <div className={styles.tableWrap}>
                <table className={styles.table}>
                    <thead>
                        <tr>
                            <th>Caso</th>
                            <th>Estado</th>
                            <th>Sub-tareas</th>
                            <th>Prioridad</th>
                            <th>Creado</th>
                            <th>Duración</th>
                        </tr>
                    </thead>
                    <tbody>
                        {visible.length === 0 && (
                            <tr><td colSpan={6} className={styles.empty}>
                                {cases.length === 0 ? 'Todavía no hay casos. Creá uno con "+ Nuevo caso".' : 'Ningún caso con ese estado.'}
                            </td></tr>
                        )}
                        {visible.map(c => {
                            const d = durationOf(c)
                            return (
                                <tr
                                    key={c.id}
                                    className={`${styles.row} ${selectedId === c.id ? styles.rowSelected : ''}`}
                                    onClick={() => onOpen(c.id)}
                                >
                                    <td>
                                        <div className={styles.name}>{c.name || '(sin nombre)'}</div>
                                        <div className={styles.id}>{c.id.slice(0, 8)}</div>
                                    </td>
                                    <td><StatusBadge status={c.status} /></td>
                                    <td className={styles.count}>{c.total_jobs}</td>
                                    <td className={styles.count}>{c.priority}</td>
                                    <td className={styles.muted}>{fmtTime(c.created_at)}</td>
                                    <td className={styles.muted}>
                                        {d == null ? '—' : fmtSeconds(d)}{!isTerminal(c.status) && d != null ? ' …' : ''}
                                    </td>
                                </tr>
                            )
                        })}
                    </tbody>
                </table>
            </div>
        </div>
    )
}
