import styles from './QueueDepth.module.css'

const POOLS = ['video', 'audio', 'metadata']

export default function QueueDepth({ queue_depth }) {
    const byPool = queue_depth?.by_pool || {}
    const total = (queue_depth?.high ?? 0) + (queue_depth?.normal ?? 0) + (queue_depth?.low ?? 0)
    const load = total > 1000 ? { label: 'crítica', tone: 'red' } : total > 200 ? { label: 'alta', tone: 'yellow' } : { label: 'normal', tone: 'green' }
    const maxPool = Math.max(1, ...POOLS.map(p => byPool[p] ?? 0))

    return (
        <div className={`card ${styles.wrap}`}>
            <div className={styles.header}>
                <span className={`chip chip-${load.tone}`}>carga {load.label}</span>
                <span className={styles.total}><b>{total}</b> en espera</span>
            </div>
            <div className={styles.pools}>
                {POOLS.map(p => {
                    const n = byPool[p] ?? 0
                    return (
                        <div key={p} className={styles.row} title={'sub-tareas esperando un worker del pool ' + p}>
                            <span className={'chip pool pool-' + p}>{p}</span>
                            <div className={styles.track}>
                                <div className={styles.fill} style={{ width: `${(n / maxPool) * 100}%`, background: `var(--pool-${p})`, opacity: n > 0 ? 1 : 0 }} />
                            </div>
                            <span className={styles.count}>{n}</span>
                        </div>
                    )
                })}
            </div>
            <div className={styles.prio}>
                <span>prioridad alta <b>{queue_depth?.high ?? 0}</b></span>
                <span>normal <b>{queue_depth?.normal ?? 0}</b></span>
                <span>baja <b>{queue_depth?.low ?? 0}</b></span>
            </div>
        </div>
    )
}
