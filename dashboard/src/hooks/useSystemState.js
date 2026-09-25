import { useState, useEffect, useRef, useCallback } from 'react'
import { api } from '../api'

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

                const allJobs = Array.isArray(data.jobs) ? data.jobs : []
                let filteredJobs = allJobs

                if (refreshTimeRef.current) {
                    filteredJobs = allJobs.filter(job => {
                        const jobCreatedAt = new Date(job.created_at).getTime()
                        return jobCreatedAt > refreshTimeRef.current
                    })
                }

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

                const liveJobs = filteredJobs.filter(job => 
                    job.status === 'pending' || job.status === 'assigned' || job.status === 'running'
                )

                setState({
                    workers: (Array.isArray(data.workers) ? [...data.workers] : []).sort((a, b) =>
                        (a.registered_at || '').localeCompare(b.registered_at || '') || a.id.localeCompare(b.id, 'es')),
                    jobs: liveJobs,
                    stats,
                    queue_depth,
                    by_case: Array.isArray(data.by_case) ? data.by_case : [],
                })
            } catch {
            }
        }

        ws.onclose = () => {
            setConnected(false)
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
        refreshTimeRef.current = Date.now()
        setState(prev => ({
            ...prev,
            jobs: [],
            stats: { pending: 0, assigned: 0, running: 0, completed: 0, failed: 0 },
            queue_depth: { high: 0, normal: 0, low: 0, by_pool: {} },
        }))
    }, [])

    return { ...state, connected, refresh }
}