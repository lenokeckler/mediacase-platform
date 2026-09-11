import styles from './WorkerCard.module.css'

function Bar({ value, max = 100, color }) {
    const pct = Math.min(100, Math.max(0, (value / max) * 100))
    return (
        <div className={styles.barTrack}>
            <div className={styles.barFill} style={{ width: `${pct}%`, background: color }} />
        </div>
    )
}

const STATUS_COLOR = {
    idle: 'var(--green)',
    busy: 'var(--yellow)',
    offline: 'var(--muted)',
}

export default function WorkerCard({ worker }) {
    const statusColor = STATUS_COLOR[worker.status] || 'var(--muted)'
    const secondsAgo = Math.round((Date.now() - new Date(worker.last_seen).getTime()) / 1000)
    const isStale = secondsAgo > 15

    return (
        <div className={`${styles.card} ${isStale ? styles.stale : ''}`}>
            <div className={styles.header}>
                <span className={styles.dot} style={{ background: isStale ? 'var(--red)' : statusColor }} />
                <span className={styles.id}>{worker.id}</span>
                <span className={styles.status} style={{ color: isStale ? 'var(--red)' : statusColor }}>
                    {isStale ? 'offline' : worker.status}
                </span>
            </div>

            <div className={styles.metric}>
                <span className={styles.label}>CPU</span>
                <Bar value={worker.cpu_percent} color={worker.cpu_percent > 80 ? 'var(--red)' : 'var(--accent)'} />
                <span className={styles.value}>{worker.cpu_percent?.toFixed(1)}%</span>
            </div>

            <div className={styles.metric}>
                <span className={styles.label}>RAM</span>
                <Bar value={worker.mem_percent} color={worker.mem_percent > 85 ? 'var(--red)' : 'var(--blue)'} />
                <span className={styles.value}>{worker.mem_percent?.toFixed(1)}%</span>
            </div>

            <div className={styles.footer}>
                <span>
                    <strong>{worker.active_jobs}</strong> active jobs
                </span>
                <span className={styles.muted}>seen {secondsAgo}s ago</span>
            </div>
        </div>
    )
}