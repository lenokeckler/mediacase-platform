import { useEffect, useRef } from 'react'

// Selección por arrastre sobre una lista de filas (p. ej. el dataset del formulario de casos):
// pointerdown en una fila decide si el arrastre selecciona o deselecciona (toma el estado
// contrario al de esa fila), y cada fila sobre la que pasa el puntero mientras está presionado
// toma ese mismo estado. Un clic simple sin arrastre solo alterna esa fila, porque el cambio ya
// ocurre en pointerdown. Shift+clic selecciona/deselecciona el rango entre la última fila tocada
// y esta, con el mismo estado.
export function useDragSelect({ orderedKeys, isSelected, setSelected, setManySelected }) {
    const dragValue = useRef(null) // true/false mientras se arrastra; null si no hay arrastre
    const lastIndex = useRef(null) // última fila tocada, ancla del rango con shift

    useEffect(() => {
        function stopDrag() {
            if (dragValue.current === null) return
            dragValue.current = null
            document.body.style.userSelect = ''
        }
        window.addEventListener('pointerup', stopDrag)
        window.addEventListener('pointercancel', stopDrag)
        return () => {
            window.removeEventListener('pointerup', stopDrag)
            window.removeEventListener('pointercancel', stopDrag)
            document.body.style.userSelect = ''
        }
    }, [])

    function onPointerDown(key, index, e) {
        // Libera la captura implícita del puntero (touch) para poder recibir pointerenter de
        // las demás filas mientras se arrastra el dedo, no solo de la fila donde empezó.
        if (e.pointerId != null && e.currentTarget?.releasePointerCapture) {
            try { e.currentTarget.releasePointerCapture(e.pointerId) } catch { /* aún no capturado */ }
        }
        if (e.shiftKey && lastIndex.current != null) {
            const from = Math.min(lastIndex.current, index)
            const to = Math.max(lastIndex.current, index)
            setManySelected(orderedKeys.slice(from, to + 1), !isSelected(key))
            lastIndex.current = index
            return
        }
        const value = !isSelected(key)
        dragValue.current = value
        lastIndex.current = index
        setSelected(key, value)
        document.body.style.userSelect = 'none'
    }

    function onPointerEnter(key) {
        if (dragValue.current === null) return
        setSelected(key, dragValue.current)
    }

    return { onPointerDown, onPointerEnter }
}
