import { useEffect, useMemo, useState } from 'react'
import { api, fileTypeOf, fmtBytes, extOf, targetsFor as catalogTargetsFor, defaultTargetFor, isEnrichOp, DEFAULT_CATALOG, OPERATION_LABEL, OPERATION_HELP, ACCEPT_EXTENSIONS } from '../api'
import EnrichmentEditor from './EnrichmentEditor'
import styles from './SubmitCasePanel.module.css'

const PRIORITIES = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]

// Un caso: nombre + prioridad + archivos. Los archivos pueden venir de esta PC (se suben al
// bucket dataset) o del dataset ya cargado. La operación la decide el coordinador; aquí solo
// se muestra cuál va a elegir y se permite cambiarla entre las válidas para ese tipo.
export default function SubmitCasePanel({ onCreated, onClose }) {
    const [name, setName] = useState('')
    const [priority, setPriority] = useState(5)
    const [local, setLocal] = useState([])        // File[] de esta PC
    const [chosen, setChosen] = useState([])      // {key, type, size, source:'dataset'|'local', operation?}
    const [dataset, setDataset] = useState([])
    const [search, setSearch] = useState('')
    const [busy, setBusy] = useState(false)
    const [error, setError] = useState(null)
    // Catálogo autoritativo del coordinador: qué operaciones y formatos acepta por tipo.
    const [catalog, setCatalog] = useState(DEFAULT_CATALOG)

    useEffect(() => {
        api.listDataset().then(setDataset).catch(e => setError(e.message))
        api.getCatalog().then(setCatalog).catch(() => {})
    }, [])
    const opsFor = (type) => catalog.ops_by_type[type] || []
    const targetsFor = (op, filename) => catalogTargetsFor(catalog, op, filename)

    const datasetFiltered = useMemo(
        () => dataset.filter(d => d.type !== 'other' && d.key.toLowerCase().includes(search.toLowerCase())),
        [dataset, search],
    )

    function addLocal(e) {
        const files = Array.from(e.target.files || [])
        const bad = files.find(f => !fileTypeOf(f.name))
        if (bad) { setError(`"${bad.name}": formato no soportado`); return }
        setError(null)
        setLocal(prev => [...prev, ...files])
        setChosen(prev => [
            ...prev,
            ...files.map(f => ({ key: f.name, type: fileTypeOf(f.name), size: f.size, source: 'local' })),
        ])
        e.target.value = ''
    }

    function toggleDataset(item) {
        setChosen(prev => prev.some(c => c.key === item.key && c.source === 'dataset')
            ? prev.filter(c => !(c.key === item.key && c.source === 'dataset'))
            : [...prev, { key: item.key, type: item.type, size: item.size_bytes, source: 'dataset' }])
    }

    function remove(i) {
        const c = chosen[i]
        if (c.source === 'local') setLocal(prev => prev.filter(f => f.name !== c.key))
        setChosen(prev => prev.filter((_, k) => k !== i))
    }

    // Al cambiar la operación se vuelve al destino por defecto de esa operación; los recursos
    // asociados se conservan por si vuelve a "enriquecer".
    function setOp(i, op) {
        setChosen(prev => prev.map((c, k) => k === i ? { ...c, operation: op || undefined, target: undefined, width: undefined } : c))
    }
    function setEnrichment(i, patch) {
        setChosen(prev => prev.map((c, k) => k === i ? { ...c, enrichment: { ...(c.enrichment || {}), ...patch } } : c))
    }
    // Artista, álbum y fecha suelen ser los mismos para todo el caso: se copian a los demás enriquecidos.
    function applyEnrichmentToAll(i) {
        const src = chosen[i].enrichment || {}
        const shared = { artist: src.artist, album: src.album, date: src.date }
        setChosen(prev => prev.map((c, k) => {
            if (k === i || !isEnrichOp(c.operation)) return c
            return { ...c, enrichment: { ...(c.enrichment || {}), ...shared } }
        }))
    }
    const enrichCount = chosen.filter(c => isEnrichOp(c.operation)).length
    function setTarget(i, target) {
        setChosen(prev => prev.map((c, k) => k === i ? { ...c, target: target || undefined } : c))
    }
    function setWidth(i, width) {
        setChosen(prev => prev.map((c, k) => k === i ? { ...c, width: width ? Number(width) : undefined } : c))
    }

    async function submit() {
        setBusy(true)
        setError(null)
        try {
            let uploadedKeys = {}
            if (local.length) {
                const res = await api.uploadFiles(local)
                local.forEach((f, i) => { uploadedKeys[f.name] = res.keys[i] })
            }
            const files = chosen.map(c => ({
                key: c.source === 'local' ? uploadedKeys[c.key] : c.key,
                ...(c.operation ? { operation: c.operation } : {}),
                ...(c.target ? { target: c.target } : {}),
                ...(c.width ? { width: c.width } : {}),
                ...(isEnrichOp(c.operation) && hasEnrichment(c.enrichment) ? { enrichment: cleanEnrichment(c.enrichment) } : {}),
            }))
            const created = await api.submitCase(name.trim(), priority, files)
            onCreated(created.id)
        } catch (e) {
            setError(e.message)
        } finally {
            setBusy(false)
        }
    }

    return (
        <div className={styles.panel}>
            <div className={styles.header}>
                <span className={styles.title}>Nuevo caso</span>
                <button className={styles.closeBtn} onClick={onClose} title="Cerrar">✕</button>
            </div>

            <div className={styles.grid}>
                <label className={styles.field}>
                    <span className={styles.label}>Nombre del caso</span>
                    <input className={styles.input} value={name} onChange={e => setName(e.target.value)}
                        placeholder="p. ej. boda-garcia, sesion-3, lote-2026-09" />
                </label>
                <label className={styles.field}>
                    <span className={styles.label}>Prioridad</span>
                    <select className={styles.select} value={priority} onChange={e => setPriority(Number(e.target.value))}>
                        {PRIORITIES.map(p => <option key={p} value={p}>{p}{p >= 8 ? ' (alta)' : p <= 3 ? ' (baja)' : ''}</option>)}
                    </select>
                </label>
            </div>

            <div className={styles.sources}>
                <div className={styles.source}>
                    <span className={styles.sourceTitle}>Subir desde esta PC</span>
                    <input className={styles.fileInput} type="file" multiple onChange={addLocal}
                        accept={ACCEPT_EXTENSIONS} />
                    <span className={styles.hint}>Video (mp4, mkv, mov, webm, avi…), audio (mp3, wav, flac, aac, ogg, aiff, dsf…) o imagen (jpg, png, webp…). Se suben al enviar el caso.</span>
                </div>
                <div className={styles.source}>
                    <span className={styles.sourceTitle}>Elegir del dataset ({dataset.length})</span>
                    <input className={styles.input} placeholder="buscar…" value={search} onChange={e => setSearch(e.target.value)} />
                    <div className={styles.datasetList}>
                        {datasetFiltered.slice(0, 200).map(d => {
                            const on = chosen.some(c => c.key === d.key && c.source === 'dataset')
                            return (
                                <label key={d.key} className={styles.datasetItem}>
                                    <input type="checkbox" checked={on} onChange={() => toggleDataset(d)} />
                                    <span className={styles.type}>{d.type}</span>
                                    <span>{d.key}</span>
                                    <span className={styles.hint}>{fmtBytes(d.size_bytes)}</span>
                                </label>
                            )
                        })}
                        {datasetFiltered.length === 0 && <span className={styles.hint}>nada que mostrar</span>}
                    </div>
                </div>
            </div>

            {chosen.length > 0 && (
                <div className={styles.chosen}>
                    <span className={styles.label}>Archivos del caso ({chosen.length}) — el coordinador decide la operación por tipo; puede cambiarla</span>
                    <div className={styles.chosenHead}><span>Archivo</span><span>Operación</span><span>Salida</span><span /></div>
                    {chosen.map((c, i) => {
                        const ops = opsFor(c.type)
                        const op = c.operation || ops[0]
                        const targets = targetsFor(op, c.key)
                        const autoTarget = defaultTargetFor(catalog, op, c.key)
                        const target = c.target || autoTarget
                        const isThumb = op === 'thumbnail'
                        const isEnrich = isEnrichOp(op)
                        return (
                            <div key={`${c.source}-${c.key}-${i}`} className={`${styles.chosenRow} ${isEnrich ? styles.chosenRowOpen : ''}`} title={OPERATION_HELP[op]}>
                                <span className={styles.chosenFile}>
                                    <span className={`chip pool pool-${catalog.pool_by_op[op] || 'metadata'}`}>{c.type}</span>
                                    <span className={styles.chosenName}>{c.key}</span>
                                    <span className={styles.hint}>{fmtBytes(c.size)}{c.source === 'local' ? ' · esta PC' : ''}</span>
                                </span>
                                <select className={styles.select} value={c.operation || ''} onChange={e => setOp(i, e.target.value)} title="Operación">
                                    <option value="">{OPERATION_LABEL[ops[0]]} (automática)</option>
                                    {ops.slice(1).map(o => <option key={o} value={o}>{OPERATION_LABEL[o]}</option>)}
                                </select>
                                <span className={styles.targetCell}>
                                    <span className="mono">{extOf(c.key)} →</span>
                                    {targets.length > 1 ? (
                                        <select className={styles.select} value={c.target || ''} onChange={e => setTarget(i, e.target.value)} title="Formato de salida">
                                            <option value="">{autoTarget.toUpperCase()} (automático)</option>
                                            {targets.filter(t => t !== autoTarget).map(t => <option key={t} value={t}>{t.toUpperCase()}</option>)}
                                        </select>
                                    ) : <span className="mono">{(target || '').toUpperCase()}</span>}
                                    {isThumb && (
                                        <select className={styles.select} value={c.width || ''} onChange={e => setWidth(i, e.target.value)} title="Ancho de la miniatura">
                                            {catalog.thumbnail_widths.map((w, k) => <option key={w} value={k === 0 ? '' : w}>{w} px</option>)}
                                        </select>
                                    )}
                                </span>
                                <button className={styles.rmBtn} onClick={() => remove(i)} title="Quitar">✕</button>
                                {isEnrich && (
                                    <EnrichmentEditor
                                        value={c.enrichment || {}}
                                        kind={c.type}
                                        filename={c.key}
                                        caseName={name}
                                        sameFormat={target === extOf(c.key)}
                                        target={target}
                                        onChange={patch => setEnrichment(i, patch)}
                                        onApplyToAll={enrichCount > 1 ? () => applyEnrichmentToAll(i) : null}
                                    />
                                )}
                            </div>
                        )
                    })}
                </div>
            )}

            {error && <div className={styles.error}>{error}</div>}

            <div className={styles.footer}>
                <button className={styles.submitBtn} disabled={busy || chosen.length === 0} onClick={submit}>
                    {busy ? 'Enviando…' : `Enviar caso (${chosen.length} archivo${chosen.length === 1 ? '' : 's'})`}
                </button>
                <span className={styles.hint}>El caso se acepta entero o se rechaza entero; el error dice qué archivo no sirve.</span>
            </div>
        </div>
    )
}

function hasEnrichment(e) { return !!e && Object.values(e).some(v => (v || '').trim() !== '') }
function cleanEnrichment(e) {
    const out = {}
    for (const [k, v] of Object.entries(e)) if ((v || '').trim() !== '') out[k] = v.trim()
    return out
}
