import styles from './QueueDepth.module.css'

const COLORS = { high: '#ef4444', normal: '#6366f1', low: '#22c55e' }

export default function QueueDepth({ queue_depth }) {
    const total = (queue_depth?.high ?? 0) + (queue_depth?.normal ?? 0) + (queue_depth?.low ?? 0)

    // Load status logic
    let status = { label: 'Normal', color: 'var(--green)' }
    if (total > 1000) {
        status = { label: 'Critical', color: 'var(--red)' }
    } else if (total > 200) {
        status = { label: 'High', color: 'var(--yellow)' }
    }

    return (
        <div className={styles.wrap}>
            <div className={styles.header}>
                <div className={styles.titleGroup}>
                    <span className={styles.title}>Queue Depth</span>
                    <span className={styles.statusBadge} style={{ background: status.color }}>
                        {status.label}
                    </span>
                </div>
                <span className={styles.total}>{total} pending</span>
            </div>
            <div className={styles.progressTrack}>
                <div 
                    className={styles.progressFill} 
                    style={{ 
                        width: `${Math.min(100, (total / 1500) * 100)}%`,
                        background: status.color 
                    }} 
                />
            </div>
        </div>
    )
}