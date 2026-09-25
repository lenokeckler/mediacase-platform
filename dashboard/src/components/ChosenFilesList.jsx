import { useMemo } from 'react'
import { extOf, fmtBytes, isEnrichOp, OPERATION_LABEL, OPERATION_HELP, TYPE_LABEL, targetsFor as catalogTargetsFor, defaultTargetFor } from '../api'
import EnrichmentEditor from './EnrichmentEditor'
import styles from './ChosenFilesList.module.css'

const TYPE_ORDER = ['video', 'audio', 'image']
const TYPE_LABEL_PLURAL = { video: 'videos', audio: 'audios', image: 'imágenes' }
// "todos los videos" pero "todas las imágenes".
const ALL_OF_TYPE = { video: 'todos los videos', audio: 'todos los audios', image: 'todas las imágenes' }

// "Archivos del caso" agrupados por tipo. Cada grupo puede copiar la operación y el formato de
// salida de su primer archivo a los demás del mismo tipo (solo si ese destino es válido para
// cada uno — misma regla que el coordinador en internal/cases/router.go, vía targetsFor).
export default function ChosenFilesList({ chosen, catalog, caseName, actions }) {
    const groups = useMemo(() => {
        const byType = {}
        chosen.forEach((c, i) => { (byType[c.type] || (byType[c.type] = [])).push(i) })
        return byType
    }, [chosen])
    const enrichCount = chosen.filter(c => isEnrichOp(c.operation)).length
    const opsFor = (type) => catalog.ops_by_type[type] || []

    return (
        <div className={styles.wrap}>
            <span className={styles.label}>
                Archivos del caso ({chosen.length}) — el coordinador decide la operación por tipo; puede cambiarla
            </span>
            {TYPE_ORDER.filter(t => groups[t]?.length).map(type => (
                <div key={type} className={styles.group}>
                    <div className={styles.groupHead}>
                        <span className={styles.groupTitle}>{TYPE_LABEL_PLURAL[type]} ({groups[type].length})</span>
                        {groups[type].length > 1 && (
                            <button type="button" className="btn btn-sm" onClick={() => actions.applyOpToType(type, groups[type][0])}
                                title={`Copia la operación y el formato de salida del primer archivo al resto de ${TYPE_LABEL_PLURAL[type]} del caso`}>
                                Aplicar a {ALL_OF_TYPE[type]}
                            </button>
                        )}
                    </div>
                    <div className={styles.rowHead}><span>Archivo</span><span>Operación</span><span>Salida</span><span /></div>
                    {groups[type].map(i => (
                        <ChosenRow key={i} i={i} c={chosen[i]} catalog={catalog} caseName={caseName} ops={opsFor(type)}
                            enrichCount={enrichCount} actions={actions} />
                    ))}
                </div>
            ))}
        </div>
    )
}

function ChosenRow({ i, c, catalog, caseName, ops, enrichCount, actions }) {
    const op = c.operation || ops[0]
    const targets = catalogTargetsFor(catalog, op, c.key)
    const autoTarget = defaultTargetFor(catalog, op, c.key)
    const target = c.target || autoTarget
    const isThumb = op === 'thumbnail'
    const isEnrich = isEnrichOp(op)
    return (
        <div className={`${styles.row} ${isEnrich ? styles.rowOpen : ''}`} title={OPERATION_HELP[op]}>
            <span className={styles.file}>
                <span className={`chip pool pool-${catalog.pool_by_op[op] || 'metadata'}`}>{TYPE_LABEL[c.type] || c.type}</span>
                <span className={styles.name}>{c.key}</span>
                <span className={styles.hint}>{fmtBytes(c.size)}{c.source === 'local' ? ' · esta PC' : ''}</span>
            </span>
            <select className="select" value={c.operation || ''} onChange={e => actions.setOp(i, e.target.value)} title="Operación">
                <option value="">{OPERATION_LABEL[ops[0]]} (automática)</option>
                {ops.slice(1).map(o => <option key={o} value={o}>{OPERATION_LABEL[o]}</option>)}
            </select>
            <span className={styles.targetCell}>
                <span className="mono">{extOf(c.key)} →</span>
                {targets.length > 1 ? (
                    <select className="select" value={c.target || ''} onChange={e => actions.setTarget(i, e.target.value)} title="Formato de salida">
                        <option value="">{autoTarget.toUpperCase()} (automático)</option>
                        {targets.filter(t => t !== autoTarget).map(t => <option key={t} value={t}>{t.toUpperCase()}</option>)}
                    </select>
                ) : <span className="mono">{(target || '').toUpperCase()}</span>}
                {isThumb && (
                    <select className="select" value={c.width || ''} onChange={e => actions.setWidth(i, e.target.value)} title="Ancho de la miniatura">
                        {catalog.thumbnail_widths.map((w, k) => <option key={w} value={k === 0 ? '' : w}>{w} px</option>)}
                    </select>
                )}
            </span>
            <button className={styles.rmBtn} onClick={() => actions.remove(i)} title="Quitar">✕</button>
            {isEnrich && (
                <EnrichmentEditor
                    value={c.enrichment || {}}
                    kind={c.type}
                    filename={c.key}
                    caseName={caseName}
                    sameFormat={target === extOf(c.key)}
                    target={target}
                    onChange={patch => actions.setEnrichment(i, patch)}
                    onApplyToAll={enrichCount > 1 ? () => actions.applyEnrichmentToAll(i) : null}
                />
            )}
        </div>
    )
}
