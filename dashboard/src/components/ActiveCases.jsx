import StatusBadge from './StatusBadge'
import styles from './ActiveCases.module.css'

const SEGMENTS = [
    { key: 'completed', label: 'listas', color: 'var(--green)' },
    { key: 'running', label: 'en ejecución', color: 'var(--accent)' },
    { key: 'pending', label: 'en espera', color: 'var(--yellow)' },
    { key: 'failed', label: 'fallidas', color: 'var(--red)' },
]

export default function ActiveCases({ byCase, onOpen }) {
    const cases = Array.isArray(byCase) ? byCase : []
    return (
        <div className={`card ${styles.wrap}`}>
            {cases.length === 0 && <div className={styles.empty}>Ningún caso abierto ahora mismo.</div>}
            {cases.length > 0 && (
                <div className={styles.legend}>
                    {SEGMENTS.map(s => (
                        <span key={s.key} className={styles.legendItem}><i className={styles.dot} style={{ background: s.color }} />{s.label}</span>
                    ))}
                </div>
            )}
            <div className={styles.list}>
                {cases.map(c => {
                    const total = Math.max(1, c.total ?? 0)
                    return (
                        <button key={c.case_id} className={styles.row} onClick={() => onOpen?.(c.case_id)} title="Ver el caso">
                            <div className={styles.rowHead}>
                                <span className={styles.name}>{c.name || c.case_id.slice(0, 8)}</span>
                                <StatusBadge status={c.status} />
                                <span className={styles.counts}>
                                    {c.completed}/{c.total} listas · {c.running} ejec. · {c.pending} espera{c.failed > 0 ? ` · ${c.failed} fallidas` : ''}
                                </span>
                            </div>
                            <div className={styles.bar}>
                                {SEGMENTS.map(s => (
                                    <div key={s.key} className={styles.seg} style={{ width: `${((c[s.key] ?? 0) / total) * 100}%`, background: s.color }} />
                                ))}
                            </div>
                        </button>
                    )
                })}
            </div>
        </div>
    )
}
