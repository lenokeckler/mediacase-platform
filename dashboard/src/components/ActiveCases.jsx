import StatusBadge from './StatusBadge'
import styles from './ActiveCases.module.css'

// Consigna (monitoreo): "sub-tareas activas o en espera, agrupadas por caso".
// Una fila por caso abierto con sus sub-tareas por estado; llega en el snapshot del WebSocket.
const SEGMENTS = [
    { key: 'completed', label: 'listas', color: '#22c55e' },
    { key: 'running', label: 'en ejecución', color: '#6366f1' },
    { key: 'pending', label: 'en espera', color: '#eab308' },
    { key: 'failed', label: 'fallidas', color: '#ef4444' },
]

export default function ActiveCases({ byCase, onOpen }) {
    const cases = Array.isArray(byCase) ? byCase : []
    const totals = SEGMENTS.reduce((acc, s) => ({ ...acc, [s.key]: cases.reduce((n, c) => n + (c[s.key] ?? 0), 0) }), {})

    return (
        <div className={styles.wrap}>
            <div className={styles.header}>
                <span className={styles.title}>Casos activos</span>
                <span className={styles.summary}>
                    {cases.length === 0
                        ? 'ningún caso abierto'
                        : `${cases.length} caso${cases.length === 1 ? '' : 's'} · ${totals.running} en ejecución · ${totals.pending} en espera`}
                </span>
            </div>
            {cases.length > 0 && (
                <div className={styles.legend}>
                    {SEGMENTS.map(s => (
                        <span key={s.key} className={styles.legendItem}>
                            <i className={styles.dot} style={{ background: s.color }} />{s.label}
                        </span>
                    ))}
                </div>
            )}
            <div className={styles.list}>
                {cases.map(c => {
                    const total = Math.max(1, c.total ?? 0)
                    return (
                        <div key={c.case_id} className={styles.row} onClick={() => onOpen?.(c.case_id)} title="Ver el caso">
                            <div className={styles.rowHead}>
                                <span className={styles.name}>{c.name || c.case_id.slice(0, 8)}</span>
                                <StatusBadge status={c.status} />
                                <span className={styles.counts}>
                                    {c.completed}/{c.total} listas · {c.running} ejec. · {c.pending} espera{c.failed > 0 ? ` · ${c.failed} fallidas` : ''}
                                </span>
                            </div>
                            <div className={styles.bar}>
                                {SEGMENTS.map(s => (
                                    <div key={s.key} className={styles.seg}
                                        style={{ width: `${((c[s.key] ?? 0) / total) * 100}%`, background: s.color }} />
                                ))}
                            </div>
                        </div>
                    )
                })}
            </div>
        </div>
    )
}
