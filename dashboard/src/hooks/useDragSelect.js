import { useEffect, useRef } from 'react'

export function useDragSelect({ orderedKeys, isSelected, setSelected, setManySelected }) {
    const dragValue = useRef(null)
    const lastIndex = useRef(null)

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
        if (e.pointerId != null && e.currentTarget?.releasePointerCapture) {
            try { e.currentTarget.releasePointerCapture(e.pointerId) } catch {  }
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
