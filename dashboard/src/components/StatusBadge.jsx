import { CASE_STATUS_LABEL, JOB_STATUS_LABEL } from '../api'
import styles from './StatusBadge.module.css'

// Colores por estado, compartidos por casos y sub-tareas.
const COLORS = {
    queued:              { bg: '#1e293b', text: '#94a3b8' },
    pending:             { bg: '#1e293b', text: '#94a3b8' },
    assigned:            { bg: '#1e3a5f', text: '#60a5fa' },
    processing:          { bg: '#1c3829', text: '#4ade80' },
    running:             { bg: '#1c3829', text: '#4ade80' },
    retrying:            { bg: '#3b0764', text: '#d8b4fe' },
    completed:           { bg: '#14532d', text: '#86efac' },
    partially_completed: { bg: '#78350f', text: '#fcd34d' },
    failed:              { bg: '#450a0a', text: '#fca5a5' },
    cancelled:           { bg: '#1f2937', text: '#9ca3af' },
}

export default function StatusBadge({ status, kind = 'case' }) {
    const c = COLORS[status] || { bg: '#1a1d2e', text: '#94a3b8' }
    const label = (kind === 'job' ? JOB_STATUS_LABEL : CASE_STATUS_LABEL)[status] || status
    return (
        <span className={styles.badge} style={{ background: c.bg, color: c.text }} title={status}>
            {label}
        </span>
    )
}
