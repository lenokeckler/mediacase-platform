import styles from './EnrichmentEditor.module.css'

export default function EnrichmentEditor({ value, kind, filename, caseName, sameFormat, target, onChange, onApplyToAll }) {
    const isAudio = kind === 'audio'
    const titleDefault = titleFromFilename(filename)
    const set = (k) => (e) => onChange({ [k]: e.target.value })

    return (
        <div className={styles.box} onClick={e => e.stopPropagation()}>
            <div className={styles.head}>
                <span className={styles.badge}>{isAudio ? '♪' : '▶'}</span>
                <div className={styles.headText}>
                    <span className={styles.title}>Recursos asociados</span>
                    <span className={styles.sub}>
                        Se integran dentro del {isAudio ? 'audio' : 'video'} · portada {isAudio ? 'con la forma de onda' : 'con el fotograma del segundo 1'} ·{' '}
                        {sameFormat ? <>se conserva el {target.toUpperCase()} sin recodificar</> : <>se re-empaqueta a {target.toUpperCase()}</>}
                    </span>
                </div>
                {onApplyToAll && (
                    <button type="button" className={styles.applyBtn} onClick={onApplyToAll}
                        title="Copia artista, álbum y fecha a los demás archivos que se van a enriquecer">
                        Aplicar a todos
                    </button>
                )}
            </div>

            <div className={styles.grid}>
                <label className={styles.field}>
                    <span className={styles.label}>Título</span>
                    <input className={styles.input} value={value.title || ''} onChange={set('title')} placeholder={titleDefault} />
                </label>
                <label className={styles.field}>
                    <span className={styles.label}>{isAudio ? 'Artista' : 'Autor'}</span>
                    <input className={styles.input} value={value.artist || ''} onChange={set('artist')} placeholder="p. ej. Equipo JLJ" />
                </label>
                <label className={styles.field}>
                    <span className={styles.label}>{isAudio ? 'Álbum / evento' : 'Evento / serie'}</span>
                    <input className={styles.input} value={value.album || ''} onChange={set('album')} placeholder={caseName.trim() || 'nombre del caso'} />
                </label>
                <label className={styles.field}>
                    <span className={styles.label}>Fecha</span>
                    <input className={styles.input} value={value.date || ''} onChange={set('date')} placeholder={String(new Date().getFullYear())} />
                </label>
                <label className={`${styles.field} ${styles.wide}`}>
                    <span className={styles.label}>Comentario</span>
                    <input className={styles.input} value={value.comment || ''} onChange={set('comment')} placeholder="sesión, lote, nota…" />
                </label>
                <label className={`${styles.field} ${styles.full}`}>
                    <span className={styles.label}>{isAudio ? 'Letra' : 'Descripción'}</span>
                    <textarea className={styles.textarea} rows={isAudio ? 4 : 2} value={value.lyrics || ''} onChange={set('lyrics')}
                        placeholder={isAudio ? 'Pegue aquí la letra; se guarda en la etiqueta de letras del archivo (la leen VLC, foobar2000, el celular…).' : 'Texto que queda en la etiqueta de descripción del video.'} />
                </label>
            </div>
        </div>
    )
}

export function titleFromFilename(name) {
    const base = (name || '').split('/').pop().replace(/\.[^.]+$/, '')
    return base.replace(/[_-]+/g, ' ').trim().replace(/\s+/g, ' ')
}
