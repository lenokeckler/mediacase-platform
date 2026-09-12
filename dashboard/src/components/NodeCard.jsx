import Sparkline from './Sparkline'
import { fmtBytes } from '../api'
import styles from './NodeCard.module.css'

// Tarjeta de un nodo al estilo Administrador de tareas > Rendimiento: CPU, memoria y cada GPU
// con porcentaje, valor absoluto y los últimos 60 s. Lo que el nodo no puede medir se muestra
// como "no disponible", nunca como 0.

const POOL_COLOR = { video: 'var(--pool-video)', audio: 'var(--pool-audio)', metadata: 'var(--pool-metadata)' }

function fmtGB(bytes) {
    if (bytes == null) return '—'
    return `${(bytes / 2 ** 30).toFixed(1)} GB`
}

function timeAgo(iso) {
    if (!iso) return ''
    const s = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000))
    return s < 60 ? `hace ${s} s` : `hace ${Math.round(s / 60)} min`
}

function Metric({ label, device, percent, secondary, series, color, unavailable }) {
    const pct = percent == null ? null : Math.round(percent)
    return (
        <div className={`${styles.metric} ${unavailable ? styles.metricOff : ''}`}>
            <div className={styles.metricHead}>
                <span className={styles.metricLabel}>{label}</span>
                {device && <span className={styles.metricDevice} title={device}>{device}</span>}
            </div>
            <div className={styles.metricBody}>
                <span className={styles.metricValue}>{pct == null ? '—' : `${pct}%`}</span>
                {secondary && <span className={styles.metricSecondary}>{secondary}</span>}
            </div>
            {unavailable
                ? <div className={styles.metricNA}>no disponible</div>
                : <Sparkline data={series} color={color} />}
        </div>
    )
}

export default function NodeCard({ worker, history, onOpen }) {
    const w = worker
    const hw = w.hardware
    const m = w.metrics
    const busy = w.status === 'busy' || w.active_jobs > 0
    const role = (w.role || 'all').toLowerCase()
    const ALL_POOLS = ['video', 'audio', 'metadata']
    const pools = w.capabilities?.length ? w.capabilities : ALL_POOLS
    const helps = ALL_POOLS.filter(p => !pools.includes(p)) // pools en los que ayuda si está libre

    const cpuPct = m ? m.cpu_percent : w.cpu_percent
    const memPct = m ? m.mem_percent : w.mem_percent
    const cpuDevice = hw?.cpu_model
        ? `${hw.cpu_model.replace(/ w\/.*$/, '')} · ${hw.cpu_cores}C/${hw.cpu_threads}T`
        : undefined
    const memSecondary = m?.mem_total_bytes ? `${fmtGB(m.mem_used_bytes)} / ${fmtGB(m.mem_total_bytes)}` : undefined
    const gpus = hw?.gpus || []
    const gpuMetric = (i) => (m?.gpus || []).find(g => g.index === i)

    return (
        <article className={`${styles.card} ${busy ? styles.busy : ''} ${onOpen ? styles.clickable : ''}`}
            onClick={onOpen ? () => onOpen(w.id) : undefined} title={onOpen ? 'Clic para ver el rendimiento en grande' : undefined}>
            <header className={styles.head}>
                <div className={styles.title}>
                    <span className={`${styles.dot} ${busy ? styles.dotBusy : styles.dotIdle}`} />
                    <span className={styles.name}>{w.id}</span>
                    {w.hostname && w.hostname.toLowerCase() !== w.id.toLowerCase() && (
                        <span className={styles.host}>{w.hostname}</span>
                    )}
                </div>
                <div className={styles.chips}>
                    {pools.map(p => (
                        <span key={p} className={styles.pool} style={{ '--c': POOL_COLOR[p] || 'var(--accent)' }} title="pool principal">{p}</span>
                    ))}
                    <span className={`${styles.status} ${busy ? styles.statusBusy : styles.statusIdle}`}>{busy ? 'ocupado' : 'libre'}</span>
                </div>
            </header>
            <div className={styles.meta}>
                {hw?.os && <span>{hw.os}</span>}
                {helps.length > 0 && <span title="Si está libre, toma sub-tareas de estos pools">ayuda en {helps.join(', ')}</span>}
                <span className={styles.seen}>visto {timeAgo(w.last_seen)}</span>
            </div>

            <div className={styles.grid}>
                <Metric label="CPU" device={cpuDevice} percent={cpuPct} series={history(w.id, 'cpu', 60)} color="var(--metric-cpu)" />
                <Metric label="Memoria" device={hw ? fmtGB(hw.mem_total_bytes) + ' instalados' : undefined} percent={memPct}
                    secondary={memSecondary} series={history(w.id, 'mem', 60)} color="var(--metric-mem)" />
                {gpus.length === 0 && (
                    <Metric label="GPU" percent={null} unavailable series={[]} color="var(--metric-gpu)" />
                )}
                {gpus.map(g => {
                    const gm = gpuMetric(g.index)
                    const parts = []
                    if (gm?.vram_used_bytes != null || g.vram_total_bytes) {
                        parts.push(`${gm?.vram_used_bytes != null ? fmtBytes(gm.vram_used_bytes) : '—'}${g.vram_total_bytes ? ' / ' + fmtBytes(g.vram_total_bytes) : ''}`)
                    }
                    if (gm?.temp_c != null) parts.push(`${Math.round(gm.temp_c)} °C`)
                    return (
                        <Metric key={g.index} label={`GPU ${g.index}`}
                            device={`${g.name}${g.integrated ? ' (integrada)' : ''}`}
                            percent={gm?.percent ?? null} unavailable={gm?.percent == null}
                            secondary={parts.join(' · ') || undefined}
                            series={history(w.id, `gpu${g.index}`, 60)} color="var(--metric-gpu)" />
                    )
                })}
            </div>

            <footer className={styles.foot}>
                <span><b>{w.active_jobs}</b> sub-tarea{w.active_jobs === 1 ? '' : 's'} activa{w.active_jobs === 1 ? '' : 's'}</span>
                {m?.disk_percent != null && <span>disco {Math.round(m.disk_percent)} %</span>}
            </footer>
        </article>
    )
}
