import { useEffect } from 'react'
import { fmtBytes, opArrow, OPERATION_LABEL } from '../api'
import StatusBadge from './StatusBadge'
import styles from './NodeDetail.module.css'

// Vista ampliada de un nodo (clic en su tarjeta): las mismas métricas que el Administrador de
// tareas pero en grande, con los últimos 5 minutos, ejes legibles, el hardware completo y las
// sub-tareas que ese nodo tiene en curso. Se cierra con Esc, con el botón o con clic afuera.

const POINTS = 300 // 5 min a 1 muestra/s

function BigChart({ label, device, percent, secondary, series, color, unavailable }) {
    const W = 720, H = 180, padL = 34, padR = 8, padT = 8, padB = 22
    const innerW = W - padL - padR, innerH = H - padT - padB
    const stepX = innerW / (POINTS - 1)
    const offset = POINTS - series.length
    const y = v => padT + innerH - (Math.min(100, Math.max(0, v)) / 100) * innerH

    let d = '', open = false
    series.forEach((v, i) => {
        const x = padL + (offset + i) * stepX
        if (v == null) { open = false; return }
        d += (open ? 'L' : 'M') + x.toFixed(1) + ',' + y(v).toFixed(1)
        open = true
    })
    const area = d && !series.includes(null) && series.length > 1
        ? `${d}L${(padL + (POINTS - 1) * stepX).toFixed(1)},${padT + innerH}L${(padL + offset * stepX).toFixed(1)},${padT + innerH}Z` : ''
    const last = series.length ? series[series.length - 1] : null
    const valid = series.filter(v => v != null)
    const avg = valid.length ? valid.reduce((a, b) => a + b, 0) / valid.length : null
    const max = valid.length ? Math.max(...valid) : null

    return (
        <section className={styles.chart}>
            <header className={styles.chartHead}>
                <div>
                    <span className={styles.chartLabel}>{label}</span>
                    {device && <span className={styles.chartDevice}>{device}</span>}
                </div>
                <div className={styles.chartNow}>
                    <span className={styles.chartValue}>{percent == null ? '—' : `${Math.round(percent)}%`}</span>
                    {secondary && <span className={styles.chartSecondary}>{secondary}</span>}
                </div>
            </header>
            {unavailable ? (
                <div className={styles.na}>no disponible en este nodo</div>
            ) : (
                <>
                    <svg viewBox={`0 0 ${W} ${H}`} width="100%" height={H} preserveAspectRatio="none" className={styles.svg} aria-label={`${label}: últimos 5 minutos`}>
                        {[0, 25, 50, 75, 100].map(p => (
                            <g key={p}>
                                <line x1={padL} x2={W - padR} y1={y(p)} y2={y(p)} stroke="var(--border)" strokeDasharray={p === 0 || p === 100 ? '' : '2 4'} />
                                <text x={padL - 6} y={y(p) + 4} fontSize="10" textAnchor="end" fill="var(--muted)">{p}%</text>
                            </g>
                        ))}
                        {[0, 60, 120, 180, 240, 300].map(s => {
                            const x = padL + (POINTS - 1 - s) * stepX
                            return (
                                <g key={s}>
                                    <line x1={x} x2={x} y1={padT} y2={padT + innerH} stroke="var(--border)" strokeDasharray="2 4" />
                                    <text x={x} y={H - 6} fontSize="10" textAnchor="middle" fill="var(--muted)">{s === 0 ? 'ahora' : `-${s / 60} min`}</text>
                                </g>
                            )
                        })}
                        {area && <path d={area} fill={color} opacity="0.15" />}
                        {d && <path d={d} fill="none" stroke={color} strokeWidth="1.8" vectorEffect="non-scaling-stroke" />}
                        {last != null && <circle cx={padL + (POINTS - 1) * stepX} cy={y(last)} r="3" fill={color} />}
                    </svg>
                    <div className={styles.chartStats}>
                        <span>promedio 5 min <b>{avg == null ? '—' : Math.round(avg) + '%'}</b></span>
                        <span>máximo <b>{max == null ? '—' : Math.round(max) + '%'}</b></span>
                        <span>muestras <b>{valid.length}</b></span>
                    </div>
                </>
            )}
        </section>
    )
}

export default function NodeDetail({ worker, history, jobs = [], onClose }) {
    useEffect(() => {
        const onKey = e => { if (e.key === 'Escape') onClose() }
        window.addEventListener('keydown', onKey)
        return () => window.removeEventListener('keydown', onKey)
    }, [onClose])
    if (!worker) return null
    const w = worker, hw = w.hardware, m = w.metrics
    const busy = w.status === 'busy' || w.active_jobs > 0
    const ALL = ['video', 'audio', 'metadata']
    const pools = w.capabilities?.length ? w.capabilities : ALL
    const helps = ALL.filter(p => !pools.includes(p))
    const mine = jobs.filter(j => j.worker_id === w.id)
    const gpus = hw?.gpus || []
    const gm = i => (m?.gpus || []).find(g => g.index === i)
    const gb = b => b == null ? '—' : `${(b / 2 ** 30).toFixed(1)} GB`

    return (
        <div className={styles.backdrop} onClick={onClose}>
            <div className={styles.modal} onClick={e => e.stopPropagation()} role="dialog" aria-modal="true" aria-label={`Rendimiento de ${w.id}`}>
                <header className={styles.head}>
                    <div className={styles.title}>
                        <span className={`${styles.dot} ${busy ? styles.dotBusy : styles.dotIdle}`} />
                        <h2>{w.id}</h2>
                        {w.hostname && w.hostname.toLowerCase() !== w.id.toLowerCase() && <span className={styles.host}>{w.hostname}</span>}
                        {pools.map(p => <span key={p} className={`chip pool pool-${p}`}>{p}</span>)}
                        <span className={`chip ${busy ? 'chip-accent' : 'chip-green'}`}>{busy ? 'ocupado' : 'libre'}</span>
                    </div>
                    <button className="btn" onClick={onClose}>Cerrar (Esc)</button>
                </header>

                <div className={styles.facts}>
                    <Fact label="Sistema">{hw?.os || '—'} · {hw?.arch || ''}</Fact>
                    <Fact label="Procesador">{hw?.cpu_model || '—'}{hw ? ` · ${hw.cpu_cores} núcleos / ${hw.cpu_threads} hilos` : ''}</Fact>
                    <Fact label="Memoria">{hw ? `${gb(hw.mem_total_bytes)} instalados` : '—'}{m ? ` · ${gb(m.mem_used_bytes)} en uso (${Math.round(m.mem_percent)}%)` : ''}</Fact>
                    <Fact label="Disco de trabajo">{m?.disk_percent != null ? `${Math.round(m.disk_percent)}% ocupado` : '—'}</Fact>
                    <Fact label="Pools">principal {pools.join(', ')}{helps.length ? ` · ayuda en ${helps.join(', ')}` : ''}</Fact>
                    <Fact label="Sub-tareas activas">{w.active_jobs}</Fact>
                </div>

                <div className={styles.charts}>
                    <BigChart label="CPU" device={hw?.cpu_model} percent={m ? m.cpu_percent : w.cpu_percent}
                        series={history(w.id, 'cpu', POINTS)} color="var(--metric-cpu)" />
                    <BigChart label="Memoria" device={hw ? `${gb(hw.mem_total_bytes)} instalados` : undefined} percent={m ? m.mem_percent : w.mem_percent}
                        secondary={m ? `${gb(m.mem_used_bytes)} / ${gb(m.mem_total_bytes)}` : undefined}
                        series={history(w.id, 'mem', POINTS)} color="var(--metric-mem)" />
                    {gpus.length === 0 && <BigChart label="GPU" unavailable series={[]} color="var(--metric-gpu)" />}
                    {gpus.map(g => {
                        const x = gm(g.index)
                        const parts = []
                        if (x?.vram_used_bytes != null || g.vram_total_bytes) parts.push(`VRAM ${x?.vram_used_bytes != null ? fmtBytes(x.vram_used_bytes) : '—'}${g.vram_total_bytes ? ' / ' + fmtBytes(g.vram_total_bytes) : ''}`)
                        if (x?.temp_c != null) parts.push(`${Math.round(x.temp_c)} °C`)
                        return (
                            <BigChart key={g.index} label={`GPU ${g.index}`} device={`${g.name}${g.integrated ? ' (integrada)' : ''} · ${g.source}`}
                                percent={x?.percent ?? null} unavailable={x?.percent == null} secondary={parts.join(' · ') || undefined}
                                series={history(w.id, `gpu${g.index}`, POINTS)} color="var(--metric-gpu)" />
                        )
                    })}
                </div>

                <section className={styles.jobs}>
                    <h3>Sub-tareas en este nodo <span className="muted">({mine.length} en curso)</span></h3>
                    {mine.length === 0 ? <p className="muted">Ninguna en curso ahora mismo.</p> : (
                        <div className="tableWrap">
                            <table className="table">
                                <thead><tr><th>Archivo</th><th>Operación</th><th>Estado</th><th>Progreso</th><th>Asignación</th></tr></thead>
                                <tbody>
                                    {mine.map(j => (
                                        <tr key={j.id}>
                                            <td>{(j.file_path || '').split(/[\\/]/).pop()}</td>
                                            <td><span className="mono">{opArrow(j)}</span> <span className="muted">{OPERATION_LABEL[j.operation]}</span></td>
                                            <td><StatusBadge status={j.status} kind="job" /></td>
                                            <td className="num">{j.status === 'running' ? `${j.progress ?? 0}%` : '—'}</td>
                                            <td>{j.assignment ? <span className={`chip ${j.assignment === 'ayuda' ? 'chip-yellow' : 'chip-blue'}`}>{j.assignment}</span> : '—'}</td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>
                        </div>
                    )}
                </section>
            </div>
        </div>
    )
}

function Fact({ label, children }) {
    return (
        <div className={styles.fact}>
            <span className={styles.factLabel}>{label}</span>
            <span className={styles.factValue}>{children}</span>
        </div>
    )
}
