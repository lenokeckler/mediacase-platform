export default function Sparkline({ data = [], max = 100, color = 'var(--accent)', width = 220, height = 44, points = 60 }) {
    const pad = 1
    const n = points
    const stepX = (width - pad * 2) / (n - 1)
    const offset = n - data.length
    const y = v => height - pad - (Math.min(max, Math.max(0, v)) / max) * (height - pad * 2)

    let d = ''
    let area = ''
    let open = false
    data.forEach((v, i) => {
        const x = pad + (offset + i) * stepX
        if (v == null) { open = false; return }
        if (!open) { d += `M${x.toFixed(1)},${y(v).toFixed(1)}`; open = true }
        else d += `L${x.toFixed(1)},${y(v).toFixed(1)}`
    })
    if (d) {
        if (!data.includes(null) && data.length > 1) {
            const x0 = pad + offset * stepX
            const x1 = pad + (n - 1) * stepX
            area = `${d}L${x1.toFixed(1)},${height - pad}L${x0.toFixed(1)},${height - pad}Z`
        }
    }
    return (
        <svg className="sparkline" viewBox={`0 0 ${width} ${height}`} width="100%" height={height} preserveAspectRatio="none" aria-hidden="true">
            <line x1={pad} x2={width - pad} y1={height / 2} y2={height / 2} stroke="var(--border)" strokeDasharray="2 3" />
            {area && <path d={area} fill={color} opacity="0.15" />}
            {d && <path d={d} fill="none" stroke={color} strokeWidth="1.5" vectorEffect="non-scaling-stroke" />}
        </svg>
    )
}
