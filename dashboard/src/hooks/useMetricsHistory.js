import { useEffect, useRef, useState } from 'react'

// Historial de 60 muestras por nodo y métrica (cpu, mem, gpu0, gpu1…) para las mini-gráficas
// tipo Administrador de tareas. Se alimenta con cada snapshot del WebSocket (~1/s); si un
// heartbeat trae la misma muestra (sampled_at igual) no se duplica.
const MAX = 60

export function useMetricsHistory(workers) {
    const ref = useRef({}) // { [workerId]: { lastSampledAt, series: { cpu: [], mem: [], gpu0: [] } } }
    const [, force] = useState(0)

    useEffect(() => {
        let changed = false
        const seen = new Set()
        for (const w of workers) {
            seen.add(w.id)
            const m = w.metrics
            const stamp = m?.sampled_at || w.last_seen
            const entry = ref.current[w.id] || (ref.current[w.id] = { lastSampledAt: null, series: {} })
            if (!stamp || entry.lastSampledAt === stamp) continue
            entry.lastSampledAt = stamp
            const push = (key, v) => {
                const s = entry.series[key] || (entry.series[key] = [])
                s.push(v == null ? null : v)
                if (s.length > MAX) s.shift()
            }
            push('cpu', m ? m.cpu_percent : w.cpu_percent)
            push('mem', m ? m.mem_percent : w.mem_percent)
            for (const g of m?.gpus || []) push(`gpu${g.index}`, g.percent)
            changed = true
        }
        // Un nodo que se fue: se olvida su historial para no crecer sin límite.
        for (const id of Object.keys(ref.current)) {
            if (!seen.has(id)) { delete ref.current[id]; changed = true }
        }
        if (changed) force(n => n + 1)
    }, [workers])

    return (workerId, key) => ref.current[workerId]?.series[key] || []
}
