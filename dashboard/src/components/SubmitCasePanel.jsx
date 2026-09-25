import { useEffect, useMemo, useState } from 'react'
import { api, fileTypeOf, targetsFor as catalogTargetsFor, isEnrichOp, DEFAULT_CATALOG, ACCEPT_EXTENSIONS } from '../api'
import DatasetPicker from './DatasetPicker'
import ChosenFilesList from './ChosenFilesList'
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

    // Claves del dataset ya elegidas, en un Set para que DatasetPicker consulte "¿está elegido?"
    // en O(1) por fila en vez de recorrer `chosen` en cada una (~600 filas).
    const chosenDatasetKeys = useMemo(
        () => new Set(chosen.filter(c => c.source === 'dataset').map(c => c.key)),
        [chosen],
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

    // items: [{key, type, size_bytes}], como los trae /api/dataset. selected=true agrega los que
    // falten, selected=false quita los que estén; usado tanto por el clic/arrastre de una fila
    // como por "marcar/desmarcar filtrados" y por el caso de prueba.
    function setDatasetSelection(items, selected) {
        setChosen(prev => {
            if (selected) {
                const existing = new Set(prev.filter(c => c.source === 'dataset').map(c => c.key))
                const toAdd = items.filter(it => !existing.has(it.key))
                    .map(it => ({ key: it.key, type: it.type, size: it.size_bytes, source: 'dataset' }))
                return toAdd.length ? [...prev, ...toAdd] : prev
            }
            const drop = new Set(items.map(it => it.key))
            return prev.filter(c => !(c.source === 'dataset' && drop.has(c.key)))
        })
    }
    function clearDatasetSelection() {
        setChosen(prev => prev.filter(c => c.source !== 'dataset'))
    }
    // Agrega los archivos de un caso de prueba predefinido con su operación/destino/enriquecimiento
    // ya fijados; el tipo y el tamaño se buscan en el dataset cargado. Si un archivo del caso de
    // prueba no está en el dataset, se omite en silencio (dataset desactualizado en este nodo).
    function loadTestCase(tc) {
        const byKey = new Map(dataset.map(d => [d.key, d]))
        setChosen(prev => {
            const existing = new Set(prev.filter(c => c.source === 'dataset').map(c => c.key))
            const additions = tc.files
                .map(f => {
                    const d = byKey.get(f.key)
                    if (!d || existing.has(f.key)) return null
                    return {
                        key: f.key, type: d.type, size: d.size_bytes, source: 'dataset',
                        ...(f.operation ? { operation: f.operation } : {}),
                        ...(f.target ? { target: f.target } : {}),
                        ...(f.width ? { width: f.width } : {}),
                        ...(f.enrichment ? { enrichment: f.enrichment } : {}),
                    }
                })
                .filter(Boolean)
            return additions.length ? [...prev, ...additions] : prev
        })
        setName(prev => prev.trim() ? prev : tc.name)
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
    function setTarget(i, target) {
        setChosen(prev => prev.map((c, k) => k === i ? { ...c, target: target || undefined } : c))
    }
    function setWidth(i, width) {
        setChosen(prev => prev.map((c, k) => k === i ? { ...c, width: width ? Number(width) : undefined } : c))
    }
    // Copia la operación y el destino (y el ancho, si aplica) del archivo `sourceIndex` a los demás
    // archivos del mismo tipo; el destino solo se copia si es válido para el archivo de llegada
    // (misma regla que el coordinador, vía targetsFor), si no vuelve al automático.
    function applyOpToType(type, sourceIndex) {
        const src = chosen[sourceIndex]
        if (!src) return
        const resolvedOp = src.operation || opsFor(type)[0]
        setChosen(prev => prev.map((c, k) => {
            if (k === sourceIndex || c.type !== type) return c
            const valid = targetsFor(resolvedOp, c.key)
            const target = src.target && valid.includes(src.target) ? src.target : undefined
            return { ...c, operation: src.operation, target, width: resolvedOp === 'thumbnail' ? src.width : undefined }
        }))
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
                    <DatasetPicker
                        dataset={dataset}
                        chosenKeys={chosenDatasetKeys}
                        onSetMany={setDatasetSelection}
                        onClear={clearDatasetSelection}
                        onLoadTestCase={loadTestCase}
                    />
                </div>
            </div>

            {chosen.length > 0 && (
                <ChosenFilesList
                    chosen={chosen}
                    catalog={catalog}
                    caseName={name}
                    actions={{ setOp, setTarget, setWidth, remove, setEnrichment, applyEnrichmentToAll, applyOpToType }}
                />
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
