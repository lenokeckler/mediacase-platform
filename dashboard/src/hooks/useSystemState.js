import { useState, useEffect, useRef, useCallback } from 'react'
import { api } from '../api'

// In dev (npm run dev), Vite proxies /ws → ws://localhost:8080/ws
// In Docker (built image), nginx proxies /ws → coordinator:8080/ws
// Either way, we always connect to /ws on the same host.
const WS_URL = typeof import.meta !== 'undefined' && import.meta.env?.VITE_WS_URL
    ? import.meta.env.VITE_WS_URL
    : `ws://${window.location.host}/ws`

const EMPTY_STATE = {
    workers: [],
    jobs: [],
    stats: { pending: 0, assigned: 0, running: 0, completed: 0, failed: 0 },
    queue_depth: { high: 0, normal: 0, low: 0, by_pool: {} },
    by_case: [],
}

export function useSystemState() {
    const [state, setState] = useState(EMPTY_STATE)
    const [connected, setConnected] = useState(false)
    const wsRef = useRef(null)
    const retryRef = useRef(null)
    const refreshTimeRef = useRef(null)

    const connect = useCallback(() => {
        if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) return

        const ws = new WebSocket(WS_URL)
        wsRef.current = ws

        ws.onopen = () => {
            setConnected(true)
            if (retryRef.current) {
                clearTimeout(retryRef.current)
                retryRef.current = null
            }
        }

        ws.onmessage = (e) => {
            try {
                const data = JSON.parse(e.data)

                // Filter all jobs by refresh time if refresh was clicked
                const allJobs = Array.isArray(data.jobs) ? data.jobs : []
                let filteredJobs = allJobs

                if (refreshTimeRef.current) {
                    filteredJobs = allJobs.filter(job => {
                        const jobCreatedAt = new Date(job.created_at).getTime()
                        return jobCreatedAt > refreshTimeRef.current
                    })
                }

                // Los contadores vienen del servidor (cuentan TODAS las sub-tareas, también las
                // terminadas, que ya no viajan en el snapshot). Tras "Limpiar" se cuentan solo las
                // vivas creadas después.
                const srv = data.stats || {}
                const stats = refreshTimeRef.current
                    ? {
                        pending: filteredJobs.filter(j => j.status === 'pending').length,
                        assigned: filteredJobs.filter(j => j.status === 'assigned').length,
                        running: filteredJobs.filter(j => j.status === 'running').length,
                        completed: 0,
                        failed: 0,
                    }
                    : {
                        pending: srv.pending ?? 0,
                        assigned: srv.assigned ?? 0,
                        running: srv.running ?? 0,
                        completed: srv.completed ?? 0,
                        failed: srv.failed ?? 0,
                    }

                // If refresh was clicked, queue_depth should be 0 (only new jobs in queue)
                // Otherwise use server data
                let queue_depth = {}
                if (refreshTimeRef.current) {
                    queue_depth = { high: 0, normal: 0, low: 0, by_pool: {} }
                } else {
                    const qd = data.queue_depth || {}
                    queue_depth = {
                        high: qd.high ?? 0,
                        normal: qd.normal ?? 0,
                        low: qd.low ?? 0,
                        by_pool: qd.by_pool || {},
                    }
                }

                // Filter jobs to only show active ones (pending, assigned, running)
                // Completed and failed jobs belong in the History tab, not Live Jobs
                const liveJobs = filteredJobs.filter(job => 
                    job.status === 'pending' || job.status === 'assigned' || job.status === 'running'
                )

                setState({
                    // Orden de llegada (registered_at) y por id si empatan: las tarjetas no se mueven.
                    workers: (Array.isArray(data.workers) ? [...data.workers] : []).sort((a, b) =>
                        (a.registered_at || '').localeCompare(b.registered_at || '') || a.id.localeCompare(b.id, 'es')),
                    jobs: liveJobs,
                    stats,
                    queue_depth,
                    by_case: Array.isArray(data.by_case) ? data.by_case : [],
                })
            } catch {
                // malformed message — ignore silently
            }
        }

        ws.onclose = () => {
            setConnected(false)
            // Reconnect after 3 seconds
            retryRef.current = setTimeout(connect, 3000)
        }

        ws.onerror = () => {
            ws.close()
        }
    }, [])

    useEffect(() => {
        connect()
        return () => {
            if (retryRef.current) clearTimeout(retryRef.current)
            if (wsRef.current) wsRef.current.close()
        }
    }, [connect])

    const refresh = useCallback(async () => {
        // Mark the refresh time - only show jobs created after this moment
        refreshTimeRef.current = Date.now()
        // Clear the live jobs display, stats, and queue depth in monitor tab
        setState(prev => ({
            ...prev,
            jobs: [],
            stats: { pending: 0, assigned: 0, running: 0, completed: 0, failed: 0 },
            queue_depth: { high: 0, normal: 0, low: 0, by_pool: {} },
        }))
    }, [])

    return { ...state, connected, refresh }
}