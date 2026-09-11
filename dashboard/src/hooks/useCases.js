import { useState, useEffect, useRef, useCallback } from 'react'
import { api, isTerminal } from '../api'

const LIST_INTERVAL_MS = 2000
const DETAIL_INTERVAL_MS = 1000

// Lista de casos (polling cada 2 s) y detalle del caso abierto (polling cada 1 s mientras
// no sea terminal; al cerrar, pide el reporte una sola vez).
export function useCases() {
    const [cases, setCases] = useState([])
    const [error, setError] = useState(null)
    const [selectedId, setSelectedId] = useState(null)
    const [selected, setSelected] = useState(null)
    const [report, setReport] = useState(null)
    const reportFor = useRef(null)

    const refresh = useCallback(async () => {
        try {
            setCases(await api.listCases())
            setError(null)
        } catch (e) {
            setError(e.message)
        }
    }, [])

    useEffect(() => {
        refresh()
        const t = setInterval(refresh, LIST_INTERVAL_MS)
        return () => clearInterval(t)
    }, [refresh])

    // Detalle del caso abierto
    useEffect(() => {
        if (!selectedId) {
            setSelected(null)
            setReport(null)
            reportFor.current = null
            return
        }
        let stop = false
        const tick = async () => {
            try {
                const c = await api.getCase(selectedId)
                if (stop) return
                setSelected(c)
                if (isTerminal(c.status) && reportFor.current !== selectedId) {
                    reportFor.current = selectedId
                    try {
                        setReport(await api.getCaseReport(selectedId))
                    } catch {
                        setReport(null)
                    }
                }
            } catch (e) {
                if (!stop) setError(e.message)
            }
        }
        tick()
        const t = setInterval(() => {
            // seguir consultando solo mientras el caso pueda cambiar
            if (!selected || !isTerminal(selected.status)) tick()
        }, DETAIL_INTERVAL_MS)
        return () => {
            stop = true
            clearInterval(t)
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [selectedId])

    const openCase = useCallback((id) => {
        setReport(null)
        reportFor.current = null
        setSelectedId(id)
    }, [])
    const closeCase = useCallback(() => setSelectedId(null), [])

    const cancel = useCallback(async (id) => {
        await api.cancelCase(id)
        await refresh()
    }, [refresh])

    return { cases, error, selected, report, openCase, closeCase, cancel, refresh }
}
