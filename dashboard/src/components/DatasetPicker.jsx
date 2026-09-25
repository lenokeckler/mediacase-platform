import { useCallback, useEffect, useMemo, useState } from 'react'
import { api, extOf, fmtBytes, TYPE_LABEL } from '../api'
import { useDragSelect } from '../hooks/useDragSelect'
import styles from './DatasetPicker.module.css'

const TYPE_ORDER = ['video', 'audio', 'image']
const TIER_ORDER = ['light', 'medium', 'heavy']
const TIER_LABEL = { light: 'Liviano', medium: 'Mediano', heavy: 'Pesado' }
const SOURCE_ORDER = ['real', 'synthetic', 'edge']
const SOURCE_LABEL = { real: 'Real', synthetic: 'Sintético', edge: 'Límite' }
const SOURCE_CHIP_TONE = { real: 'chip-green', synthetic: 'chip-gray', edge: 'chip-yellow' }

export default function DatasetPicker({ dataset, chosenKeys, onSetMany, onClear, onLoadTestCase }) {
    const [search, setSearch] = useState('')
    const [typeFilter, setTypeFilter] = useState([])
    const [formatFilter, setFormatFilter] = useState([])
    const [tierFilter, setTierFilter] = useState([])
    const [sourceFilter, setSourceFilter] = useState([])
    const [testCases, setTestCases] = useState([])

    useEffect(() => {
        let alive = true
        api.listTestCases().then(list => { if (alive) setTestCases(Array.isArray(list) ? list : []) }).catch(() => {})
        return () => { alive = false }
    }, [])

    const facets = useMemo(() => {
        const types = {}, formats = {}, tiers = {}, sources = {}
        for (const d of dataset) {
            if (d.type === 'other') continue
            types[d.type] = (types[d.type] || 0) + 1
            const fmt = extOf(d.key)
            formats[fmt] = (formats[fmt] || 0) + 1
            if (d.tier) tiers[d.tier] = (tiers[d.tier] || 0) + 1
            if (d.source) sources[d.source] = (sources[d.source] || 0) + 1
        }
        return { types, formats, tiers, sources }
    }, [dataset])
    const formats = useMemo(() => Object.keys(facets.formats).sort(), [facets])
    const hasTiers = Object.keys(facets.tiers).length > 0
    const hasSources = Object.keys(facets.sources).length > 0

    const filtered = useMemo(() => {
        const q = search.toLowerCase()
        return dataset.filter(d => {
            if (d.type === 'other') return false
            if (q && !d.key.toLowerCase().includes(q)) return false
            if (typeFilter.length && !typeFilter.includes(d.type)) return false
            if (formatFilter.length && !formatFilter.includes(extOf(d.key))) return false
            if (tierFilter.length && !tierFilter.includes(d.tier)) return false
            if (sourceFilter.length && !sourceFilter.includes(d.source)) return false
            return true
        })
    }, [dataset, search, typeFilter, formatFilter, tierFilter, sourceFilter])
    const selectedInFiltered = useMemo(() => filtered.reduce((n, d) => n + (chosenKeys.has(d.key) ? 1 : 0), 0), [filtered, chosenKeys])

    const byKey = useMemo(() => new Map(dataset.map(d => [d.key, d])), [dataset])
    const orderedKeys = useMemo(() => filtered.map(d => d.key), [filtered])
    const isSelected = useCallback(key => chosenKeys.has(key), [chosenKeys])
    const setSelected = useCallback((key, value) => {
        const item = byKey.get(key)
        if (item) onSetMany([item], value)
    }, [byKey, onSetMany])
    const setManySelected = useCallback((keys, value) => {
        const items = keys.map(k => byKey.get(k)).filter(Boolean)
        if (items.length) onSetMany(items, value)
    }, [byKey, onSetMany])
    const drag = useDragSelect({ orderedKeys, isSelected, setSelected, setManySelected })

    return (
        <div className={styles.wrap}>
            <div className={styles.searchRow}>
                <input className="input" placeholder="buscar…" value={search} onChange={e => setSearch(e.target.value)} />
                <span className={styles.count}>{filtered.length} de {dataset.length} · {chosenKeys.size} elegidos</span>
            </div>

            <div className={styles.filterGroups}>
                <FilterGroup label="Tipo" order={TYPE_ORDER} labelFor={v => TYPE_LABEL[v] || v} counts={facets.types} active={typeFilter} onToggle={v => toggleFilter(setTypeFilter, v)} />
                {formats.length > 1 && (
                    <FilterGroup label="Formato" order={formats} labelFor={v => v.toUpperCase()} counts={facets.formats} active={formatFilter} onToggle={v => toggleFilter(setFormatFilter, v)} />
                )}
                {hasTiers && (
                    <FilterGroup label="Tamaño" order={TIER_ORDER} labelFor={v => TIER_LABEL[v] || v} counts={facets.tiers} active={tierFilter} onToggle={v => toggleFilter(setTierFilter, v)} />
                )}
                {hasSources && (
                    <FilterGroup label="Origen" order={SOURCE_ORDER} labelFor={v => SOURCE_LABEL[v] || v} counts={facets.sources} active={sourceFilter} onToggle={v => toggleFilter(setSourceFilter, v)} />
                )}
            </div>

            <div className={styles.bulkActions}>
                <button type="button" className="btn btn-sm" disabled={filtered.length === 0} onClick={() => onSetMany(filtered, true)}>Marcar todo lo filtrado</button>
                <button type="button" className="btn btn-sm" disabled={selectedInFiltered === 0} onClick={() => onSetMany(filtered, false)}>Desmarcar filtrados</button>
                <button type="button" className="btn btn-sm" disabled={chosenKeys.size === 0} onClick={onClear}>Limpiar selección</button>
            </div>

            {testCases.length > 0 && (
                <label className={styles.testCaseRow}>
                    <span className={styles.testCaseLabel}>Cargar caso de prueba</span>
                    <select className="select" value="" onChange={e => {
                        const tc = testCases.find(t => t.id === e.target.value)
                        if (tc) onLoadTestCase(tc)
                    }}>
                        <option value="">elegir…</option>
                        {testCases.map(tc => (
                            <option key={tc.id} value={tc.id}>
                                {tc.name} · {tc.kind === 'heterogeneous' ? 'heterogéneo' : 'homogéneo'} · {tc.files.length} archivos
                            </option>
                        ))}
                    </select>
                </label>
            )}

            <div className={styles.list}>
                {filtered.map((d, idx) => {
                    const selected = chosenKeys.has(d.key)
                    return (
                        <div
                            key={d.key}
                            data-key={d.key}
                            className={`${styles.item} ${selected ? styles.itemSelected : ''}`}
                            onPointerDown={e => drag.onPointerDown(d.key, idx, e)}
                            onPointerEnter={() => drag.onPointerEnter(d.key)}
                        >
                            <input type="checkbox" checked={selected} readOnly tabIndex={-1} onClick={e => e.preventDefault()} />
                            <span className={styles.type}>{TYPE_LABEL[d.type] || d.type}</span>
                            <span className={styles.name}>{d.key}</span>
                            {d.source && <span className={`chip ${SOURCE_CHIP_TONE[d.source] || 'chip-gray'}`}>{SOURCE_LABEL[d.source] || d.source}</span>}
                            {d.note && <span className={styles.note} title={d.note}>ⓘ</span>}
                            <span className={styles.hint}>{fmtBytes(d.size_bytes)}</span>
                        </div>
                    )
                })}
                {filtered.length === 0 && <span className={styles.hint}>nada que mostrar</span>}
            </div>
        </div>
    )
}

function toggleFilter(setter, value) {
    setter(prev => prev.includes(value) ? prev.filter(v => v !== value) : [...prev, value])
}

function FilterGroup({ label, order, labelFor, counts, active, onToggle }) {
    const options = order.filter(v => counts[v])
    if (options.length === 0) return null
    return (
        <div className={styles.filterGroup}>
            <span className={styles.filterGroupLabel}>{label}</span>
            <div className="filters">
                {options.map(v => (
                    <button key={v} type="button" className={`filter ${active.includes(v) ? 'active' : ''}`} onClick={() => onToggle(v)}>
                        {labelFor(v)}
                        <span className="pill">{counts[v]}</span>
                    </button>
                ))}
            </div>
        </div>
    )
}
