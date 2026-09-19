import { isEnrichOp } from '../api'
import styles from './EnrichmentChips.module.css'

// Resumen de lo que una sub-tarea "enriquecer" integró en el archivo: portada (siempre), cuántas
// etiquetas y si lleva letra/descripción. El tooltip muestra los valores.
export default function EnrichmentChips({ job }) {
    if (!isEnrichOp(job.operation)) return null
    const e = job.enrichment || {}
    const tags = ['title', 'artist', 'album', 'date', 'comment'].filter(k => (e[k] || '').trim() !== '')
    const isAudio = job.operation === 'enrich_audio'
    const textLabel = isAudio ? 'letra' : 'descripción'
    const tip = [
        `portada: ${isAudio ? 'forma de onda' : 'fotograma'}`,
        ...tags.map(k => `${TAG_LABEL[k]}: ${e[k]}`),
        e.lyrics ? `${textLabel}: ${e.lyrics.length > 60 ? e.lyrics.slice(0, 60) + '…' : e.lyrics}` : null,
    ].filter(Boolean).join('\n')
    return (
        <span className={styles.chips} title={tip}>
            <span className={styles.chip}>portada</span>
            {tags.length > 0 && <span className={styles.chip}>{tags.length} etiqueta{tags.length === 1 ? '' : 's'}</span>}
            {e.lyrics && <span className={styles.chip}>{textLabel}</span>}
        </span>
    )
}

const TAG_LABEL = { title: 'título', artist: 'artista', album: 'álbum', date: 'fecha', comment: 'comentario' }
