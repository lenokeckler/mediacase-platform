import styles from './StatsBar.module.css'

// Contadores de sub-tareas de todo el sistema; el color es semántico (estado), no decorativo.
const STAT_CONFIG = [
    { key: 'pending', label: 'Pendientes', color: 'var(--gray)', hint: 'en cola, sin worker todavía' },
    { key: 'assigned', label: 'Asignadas', color: 'var(--blue)', hint: 'entregadas a un worker' },
    { key: 'running', label: 'En ejecución', color: 'var(--accent)', hint: 'ffmpeg corriendo' },
    { key: 'completed', label: 'Completadas', color: 'var(--green)', hint: 'con resultado' },
    { key: 'failed', label: 'Fallidas', color: 'var(--red)', hint: 'con error' },
]

export default function StatsBar({ stats }) {
    return (
        <div className={styles.bar}>
            {STAT_CONFIG.map(({ key, label, color, hint }) => (
                <div key={key} className={`card ${styles.stat}`} title={hint}>
                    <div className={styles.head}>
                        <span className={styles.dot} style={{ background: color }} />
                        <span className={styles.label}>{label}</span>
                    </div>
                    <div className={styles.value}>{(stats?.[key] ?? 0).toLocaleString('es')}</div>
                </div>
            ))}
        </div>
    )
}
