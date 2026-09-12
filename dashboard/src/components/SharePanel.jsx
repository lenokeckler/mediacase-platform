import { useEffect, useState } from 'react'
import { api } from '../api'
import styles from './SharePanel.module.css'

// Tarjeta "Compartir": cómo sumar otra computadora. En el mismo WiFi basta la URL de la LAN;
// desde otra red, el botón abre el túnel de Cloudflare (dos procesos que maneja el coordinador)
// y muestra la URL https. Si la red bloquea el túnel (el WiFi del TEC), el coordinador lo
// detecta y aquí se ve la pista: encender WARP.

const POLL_MS = 3000

function CopyButton({ text }) {
    const [done, setDone] = useState(false)
    const copy = async () => {
        try { await navigator.clipboard.writeText(text) } catch { /* http sin clipboard: seleccionar a mano */ }
        setDone(true)
        setTimeout(() => setDone(false), 1500)
    }
    return (
        <button className="btn btn-sm" onClick={copy} title="Copiar">
            {done ? '✓ copiado' : 'Copiar'}
        </button>
    )
}

export default function SharePanel() {
    const [info, setInfo] = useState(null)
    const [busy, setBusy] = useState(false)
    const [err, setErr] = useState('')

    const load = async () => {
        try { setInfo(await api.getShare()); setErr('') } catch (e) { setErr(e.message) }
    }
    useEffect(() => {
        load()
        const t = setInterval(load, POLL_MS)
        return () => clearInterval(t)
    }, [])

    const tunnel = info?.tunnel || { status: 'off' }
    const toggle = async () => {
        setBusy(true)
        try {
            if (tunnel.status === 'on' || tunnel.status === 'starting') await api.stopTunnel()
            else await api.startTunnel()
            await load()
        } catch (e) { setErr(e.message) } finally { setBusy(false) }
    }

    const lanUrl = info?.primary_url || ''
    const others = (info?.lan_urls || []).filter(u => u !== lanUrl)

    return (
        <div className={`card ${styles.wrap}`}>
            {err && <p className="alert alert-red">{err}</p>}

            <div className={styles.row}>
                <div className={styles.label}>Mismo WiFi</div>
                {lanUrl ? (
                    <div className={styles.urlBox}>
                        <a href={`${lanUrl}/connect`} target="_blank" rel="noreferrer" className={styles.url}>{lanUrl}/connect</a>
                        <CopyButton text={`${lanUrl}/connect`} />
                    </div>
                ) : <span className={styles.muted}>sin dirección de red detectada</span>}
                {others.length > 0 && (
                    <div className={styles.others}>otras interfaces: {others.map(u => u.replace('http://', '')).join(' · ')}</div>
                )}
            </div>

            <div className={styles.row}>
                <div className={styles.labelRow}>
                    <span className={styles.label}>Otra red (túnel)</span>
                    <span className={`chip ${{ off: 'chip-gray', starting: 'chip-yellow', on: 'chip-green', error: 'chip-red' }[tunnel.status] || 'chip-gray'}`}>
                        {{ off: 'cerrado', starting: 'abriendo…', on: 'abierto', error: 'error' }[tunnel.status] || tunnel.status}
                    </span>
                    <button
                        className={`btn ${styles.action} ${tunnel.status === 'on' ? 'btn-danger' : 'btn-primary'}`}
                        onClick={toggle}
                        disabled={busy || (info && !info.cloudflared_installed && tunnel.status !== 'on')}
                    >
                        {tunnel.status === 'on' ? 'Cerrar túnel' : tunnel.status === 'starting' ? 'Cancelar' : 'Publicar en internet'}
                    </button>
                </div>

                {tunnel.status === 'on' && (
                    <div className={styles.urlBox}>
                        <a href={`${tunnel.coordinator_url}/connect`} target="_blank" rel="noreferrer" className={styles.url}>
                            {tunnel.coordinator_url}/connect
                        </a>
                        <CopyButton text={`${tunnel.coordinator_url}/connect`} />
                    </div>
                )}
                {tunnel.status === 'on' && (
                    <div className={styles.others}>
                        esta URL cambia cada vez que se abre el túnel: el ZIP se baja con el túnel abierto
                    </div>
                )}
                {tunnel.status === 'starting' && (
                    <div className={styles.muted}>conectando con Cloudflare (hasta 45 s)…</div>
                )}
                {tunnel.status === 'error' && (
                    <div className="alert alert-red">
                        <div>{tunnel.error}</div>
                        {tunnel.hint && <div className={styles.hint}>{tunnel.hint}</div>}
                    </div>
                )}
                {info && !info.cloudflared_installed && tunnel.status === 'off' && (
                    <div className={styles.muted}>
                        falta <code>cloudflared</code> en esta máquina: <code>winget install --id Cloudflare.cloudflared</code>
                    </div>
                )}
            </div>
        </div>
    )
}
