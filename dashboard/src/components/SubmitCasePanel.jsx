import { useEffect, useMemo, useState } from 'react'
import { api, fileTypeOf, fmtBytes, OPS_BY_TYPE, OPERATION_LABEL } from '../api'
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

    useEffect(() => {
        api.listDataset().then(setDataset).catch(e => setError(e.message))
    }, [])

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

    function setOp(i, op) {
        setChosen(prev => prev.map((c, k) => k === i ? { ...c, operation: op || undefined } : c))
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
                        accept=".mp4,.mkv,.avi,.mov,.webm,.mp3,.wav,.flac,.aac,.ogg,.m4a,.jpg,.jpeg,.png,.gif,.webp,.bmp" />
                    <span className={styles.hint}>Video, audio o imagen. Se suben al repositorio de entradas al enviar el caso.</span>
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
                    <span className={styles.label}>Archivos del caso ({chosen.length}) — operación que va a decidir el coordinador</span>
                    {chosen.map((c, i) => (
                        <div key={`${c.source}-${c.key}-${i}`} className={styles.chosenRow}>
                            <span>{c.key} <span className={styles.hint}>{fmtBytes(c.size)}{c.source === 'local' ? ' · esta PC' : ''}</span></span>
                            <span className={styles.type}>{c.type}</span>
                            <select className={styles.select} value={c.operation || ''} onChange={e => setOp(i, e.target.value)}>
                                <option value="">automática: {OPERATION_LABEL[OPS_BY_TYPE[c.type][0]]}</option>
                                {OPS_BY_TYPE[c.type].map(op => <option key={op} value={op}>{OPERATION_LABEL[op]}</option>)}
                            </select>
                            <button className={styles.rmBtn} onClick={() => remove(i)} title="Quitar">✕</button>
                        </div>
                    ))}
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
