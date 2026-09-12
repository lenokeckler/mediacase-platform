import { CASE_STATUS_LABEL, JOB_STATUS_LABEL } from '../api'

// Chip de estado, compartido por casos y sub-tareas. El color es semántico y viene del tema
// (ui.css): verde = terminó bien, ámbar = parcial, rojo = falló, violeta = reintentando…
const TONE = {
    queued: 'gray', pending: 'gray',
    assigned: 'blue',
    processing: 'accent', running: 'accent',
    retrying: 'violet',
    completed: 'green',
    partially_completed: 'yellow',
    failed: 'red',
    cancelled: 'gray',
}

export default function StatusBadge({ status, kind = 'case' }) {
    const label = (kind === 'job' ? JOB_STATUS_LABEL : CASE_STATUS_LABEL)[status] || status
    return <span className={`chip chip-${TONE[status] || 'gray'}`} title={status}>{label}</span>
}
