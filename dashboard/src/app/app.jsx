import { useState } from 'react'
import { useSystemState } from '../hooks/useSystemState'
import { useCases } from '../hooks/useCases'
import WorkerCard from '../components/WorkerCard'
import JobTable from '../components/JobTable'
import StatsBar from '../components/StatsBar'
import QueueDepth from '../components/QueueDepth'
import ActiveCases from '../components/ActiveCases'
import JobHistory from '../components/JobHistory'
import CasesPanel from '../components/CasesPanel'
import SubmitCasePanel from '../components/SubmitCasePanel'
import CaseDetail from '../components/CaseDetail'
import SharePanel from '../components/SharePanel'
import styles from './app.module.css'

const TABS = ['Casos', 'Monitor', 'Historial']

function ConnectionBadge({ connected }) {
    return (
        <div className={`${styles.connBadge} ${connected ? styles.connOk : styles.connErr}`}>
            <span className={styles.connDot} />
            {connected ? 'En vivo' : 'Reconectando…'}
        </div>
    )
}

export default function App() {
    const { workers, jobs, stats, queue_depth, by_case, connected, refresh } = useSystemState()
    const casesState = useCases()
    // Pestaña inicial desde el hash (http://…:8080/#monitor) para enlazar directo al monitoreo.
    const [tab, setTab] = useState(() => ({ '#monitor': 'Monitor', '#historial': 'Historial' })[window.location.hash] || 'Casos')
    const [showNew, setShowNew] = useState(false)

    const activeCases = casesState.cases.filter(c => !['completed', 'partially_completed', 'failed', 'cancelled'].includes(c.status)).length

    return (
        <div className={styles.layout}>
            {/* ── Cabecera ── */}
            <header className={styles.header}>
                <div className={styles.headerLeft}>
                    <span className={styles.logo}>⚡ MediaCase</span>
                    <span className={styles.subtitle}>Procesamiento multimedia distribuido por casos</span>
                </div>
                <nav className={styles.tabs}>
                    {TABS.map(t => (
                        <button
                            key={t}
                            className={`${styles.tab} ${tab === t ? styles.tabActive : ''}`}
                            onClick={() => setTab(t)}
                        >
                            {t}{t === 'Casos' && activeCases > 0 ? ` (${activeCases})` : ''}
                        </button>
                    ))}
                    <a className={styles.tab} href="/connect" target="_blank" rel="noreferrer" title="Sumar otra computadora como worker">
                        Conectar esta PC ↗
                    </a>
                </nav>
                <ConnectionBadge connected={connected} />
            </header>

            <main className={styles.main}>

                {/* ── Casos ── */}
                {tab === 'Casos' && (
                    <>
                        {showNew && (
                            <section className={styles.section}>
                                <SubmitCasePanel
                                    onClose={() => setShowNew(false)}
                                    onCreated={(id) => { setShowNew(false); casesState.openCase(id); casesState.refresh() }}
                                />
                            </section>
                        )}
                        {casesState.selected && (
                            <section className={styles.section}>
                                <CaseDetail
                                    c={casesState.selected}
                                    report={casesState.report}
                                    onCancel={casesState.cancel}
                                    onClose={casesState.closeCase}
                                />
                            </section>
                        )}
                        <section className={styles.section}>
                            <div className={styles.sectionHeader}>
                                <h2 className={styles.sectionTitle}>Casos</h2>
                                <span className={styles.sectionCount}>{casesState.cases.length} en total · clic en uno para ver sus sub-tareas y su reporte</span>
                            </div>
                            <CasesPanel
                                cases={casesState.cases}
                                error={casesState.error}
                                selectedId={casesState.selected?.id}
                                onOpen={casesState.openCase}
                                onNew={() => setShowNew(true)}
                            />
                        </section>
                    </>
                )}

                {/* ── Monitor ── */}
                {tab === 'Monitor' && (
                    <>
                        <section className={styles.section}>
                            <StatsBar stats={stats} />
                        </section>

                        <section className={styles.section}>
                            <div className={styles.sectionHeader}>
                                <h2 className={styles.sectionTitle}>Nodos worker</h2>
                                <span className={styles.sectionCount}>{workers.length} conectado{workers.length === 1 ? '' : 's'}</span>
                            </div>
                            <div className={styles.workerGrid}>
                                {workers.length === 0 && (
                                    <p className={styles.empty}>No hay workers conectados. Abra <a href="/connect" target="_blank" rel="noreferrer">Conectar esta PC</a> en cualquier máquina de la red.</p>
                                )}
                                {workers.map(w => <WorkerCard key={w.id} worker={w} />)}
                                <QueueDepth queue_depth={queue_depth} />
                            </div>
                        </section>

                        <section className={styles.section}>
                            <SharePanel />
                        </section>

                        <section className={styles.section}>
                            <div className={styles.sectionHeader}>
                                <h2 className={styles.sectionTitle}>Sub-tareas agrupadas por caso</h2>
                                <span className={styles.sectionCount}>{by_case.length} caso{by_case.length === 1 ? '' : 's'} abierto{by_case.length === 1 ? '' : 's'} · clic en uno para abrirlo</span>
                            </div>
                            <ActiveCases byCase={by_case} onOpen={(id) => { casesState.openCase(id); setTab('Casos') }} />
                        </section>

                        <section className={styles.section}>
                            <div className={styles.sectionHeader}>
                                <h2 className={styles.sectionTitle}>Sub-tareas en curso</h2>
                                <span className={styles.sectionCount}>{jobs.length} activas</span>
                                <button className={styles.refreshBtn} onClick={refresh} title="Limpiar la vista">
                                    ↻ Limpiar
                                </button>
                            </div>
                            <JobTable jobs={jobs} />
                        </section>
                    </>
                )}

                {/* ── Historial ── */}
                {tab === 'Historial' && (
                    <section className={styles.section}>
                        <div className={styles.sectionHeader}>
                            <h2 className={styles.sectionTitle}>Historial de sub-tareas</h2>
                            <span className={styles.sectionCount}>Todas las sub-tareas — clic en una fila para ver el detalle</span>
                        </div>
                        <JobHistory />
                    </section>
                )}

            </main>
        </div>
    )
}
