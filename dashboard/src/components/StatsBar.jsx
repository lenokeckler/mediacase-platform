import styles from './StatsBar.module.css'

const STAT_CONFIG = [
    { key: 'pending', label: 'Pendientes', color: 'var(--muted)' },
    { key: 'assigned', label: 'Asignadas', color: 'var(--blue)' },
    { key: 'running', label: 'En ejecución', color: 'var(--accent)' },
    { key: 'completed', label: 'Completadas', color: 'var(--green)' },
    { key: 'failed', label: 'Fallidas', color: 'var(--red)' },
]

export default function StatsBar({ stats }) {
    return (
        <div className={styles.bar}>
            {STAT_CONFIG.map(({ key, label, color }) => (
                <div key={key} className={styles.stat}>
                    <span className={styles.dot} style={{ background: color }} />
                    <div>
                        <div className={styles.value} style={{ color }}>{stats?.[key] ?? 0}</div>
                        <div className={styles.label}>{label}</div>
                    </div>
                </div>
            ))}
        </div>
    )
}