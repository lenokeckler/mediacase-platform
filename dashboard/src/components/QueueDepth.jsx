import styles from './QueueDepth.module.css'

const COLORS = { high: '#ef4444', normal: '#6366f1', low: '#22c55e' }

export default function QueueDepth({ queue_depth }) {
    const total = (queue_depth?.high ?? 0) + (queue_depth?.normal ?? 0) + (queue_depth?.low ?? 0)

    // Load status logic
    let status = { label: 'normal', color: 'var(--green)' }
    if (total > 1000) {
        status = { label: 'crítica', color: 'var(--red)' }
    } else if (total > 200) {
        status = { label: 'alta', color: 'var(--yellow)' }
    }
    const byPool = queue_depth?.by_pool || {}
    const pools = ['video', 'audio', 'metadata']
    const maxPool = Math.max(1, ...pools.map(p => byPool[p] ?? 0))

    return (
        <div className={styles.wrap}>
            <div className={styles.header}>
                <div className={styles.titleGroup}>
                    <span className={styles.title}>Colas de sub-tareas</span>
                    <span className={styles.statusBadge} style={{ background: status.color }}>
                        {status.label}
                    </span>
                </div>
                <span className={styles.total}>{total} en espera</span>
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
            <div className={styles.pools}>
                {pools.map(p => (
                    <div key={p} className={styles.poolRow} title={`sub-tareas esperando un worker del pool ${p}`}>
                        <span className={styles.poolName}>{p}</span>
                        <div className={styles.poolTrack}>
                            <div className={styles.poolFill} style={{ width: `${((byPool[p] ?? 0) / maxPool) * 100}%`, background: (byPool[p] ?? 0) > 0 ? 'var(--yellow)' : 'var(--border)' }} />
                        </div>
                        <span className={styles.poolCount}>{byPool[p] ?? 0}</span>
                    </div>
                ))}
            </div>
        </div>
    )
}
