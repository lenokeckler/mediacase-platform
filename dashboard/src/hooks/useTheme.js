import { useEffect, useState } from 'react'

// Tema claro / oscuro / según el sistema. Se guarda en localStorage y se aplica como
// data-theme en <html>; index.css define las variables para cada uno.
const KEY = 'mediacase.theme'
const OPTIONS = ['system', 'light', 'dark']

function systemTheme() {
    return window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

export function useTheme() {
    const [pref, setPref] = useState(() => {
        // ?theme=light|dark en la URL manda (capturas, demos); si no, lo guardado; si no, el sistema.
        const q = new URLSearchParams(window.location.search).get('theme')
        if (OPTIONS.includes(q)) return q
        try { return OPTIONS.includes(localStorage.getItem(KEY)) ? localStorage.getItem(KEY) : 'system' } catch { return 'system' }
    })
    const effective = pref === 'system' ? systemTheme() : pref

    useEffect(() => {
        document.documentElement.dataset.theme = effective
        try { localStorage.setItem(KEY, pref) } catch { /* modo privado */ }
    }, [pref, effective])

    useEffect(() => {
        if (pref !== 'system') return
        const mq = window.matchMedia('(prefers-color-scheme: dark)')
        const onChange = () => { document.documentElement.dataset.theme = systemTheme() }
        mq.addEventListener('change', onChange)
        return () => mq.removeEventListener('change', onChange)
    }, [pref])

    // Un solo botón que alterna entre claro y oscuro (partiendo del efectivo).
    const toggle = () => setPref(effective === 'dark' ? 'light' : 'dark')
    return { theme: effective, pref, setPref, toggle }
}
