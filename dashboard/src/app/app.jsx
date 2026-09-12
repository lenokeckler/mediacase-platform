import { useEffect, useState } from 'react'
import { useSystemState } from '../hooks/useSystemState'
import { useCases } from '../hooks/useCases'
import { useMetricsHistory } from '../hooks/useMetricsHistory'
import { useTheme } from '../hooks/useTheme'
import NodeCard from '../components/NodeCard'
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

// Navegación: una entrada por pestaña; el hash de la URL la fija (http://…:8080/#monitor).
const NAV = [
    { key: 'Casos', hash: '#casos', icon: CasesIcon, hint: 'Enviar y seguir casos' },
    { key: 'Monitor', hash: '#monitor', icon: MonitorIcon, hint: 'Nodos, rendimiento y colas' },
    { key: 'Historial', hash: '#historial', icon: HistoryIcon, hint: 'Todas las sub-tareas' },
]
const TERMINAL = ['completed', 'partially_completed', 'failed', 'cancelled']

function Section({ title, count, action, children, id }) {
    return (
        <section className={styles.section} id={id}>
            <header className={styles.sectionHeader}>
                <h2 className={styles.sectionTitle}>{title}</h2>
                {count != null && <span className={styles.sectionCount}>{count}</span>}
                {action && <div className={styles.sectionAction}>{action}</div>}
            </header>
            {children}
        </section>
    )
}

export default function App() {
    const { workers, jobs, stats, queue_depth, by_case, connected, refresh } = useSystemState()
    const casesState = useCases()
    const history = useMetricsHistory(workers)
    const { theme, toggle } = useTheme()
    const [tab, setTab] = useState(() => NAV.find(n => n.hash === window.location.hash)?.key || 'Casos')
    const [showNew, setShowNew] = useState(() => window.location.hash === '#nuevo') // #nuevo abre el formulario

    // #caso=<id> abre directamente ese caso (enlaces desde el informe, Grafana o un compañero).
    useEffect(() => {
        const m = window.location.hash.match(/^#caso=([0-9a-f-]+)/)
        if (m) casesState.openCase(m[1])
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [])

    const go = (key) => {
        setTab(key)
        window.location.hash = NAV.find(n => n.key === key)?.hash || ''
    }
    const activeCases = casesState.cases.filter(c => !TERMINAL.includes(c.status)).length
    const busyNodes = workers.filter(w => w.status === 'busy' || w.active_jobs > 0).length

    return (
        <div className={styles.shell}>
            {/* ── Barra lateral ── */}
            <aside className={styles.sidebar}>
                <div className={styles.brand}>
                    <span className={styles.brandMark}>⚡</span>
                    <div>
                        <div className={styles.brandName}>MediaCase</div>
                        <div className={styles.brandSub}>Procesamiento por casos</div>
                    </div>
                </div>

                <nav className={styles.nav} aria-label="Secciones">
                    <div className={styles.navLabel}>Operación</div>
                    {NAV.map(n => (
                        <button key={n.key} className={`${styles.navItem} ${tab === n.key ? styles.navActive : ''}`}
                            onClick={() => go(n.key)} title={n.hint}>
                            <n.icon />
                            <span>{n.key}</span>
                            {n.key === 'Casos' && activeCases > 0 && <span className={styles.navBadge}>{activeCases}</span>}
                            {n.key === 'Monitor' && workers.length > 0 && <span className={styles.navBadge}>{workers.length}</span>}
                        </button>
                    ))}
                    <div className={styles.navLabel}>Nodos</div>
                    <a className={styles.navItem} href="/connect" target="_blank" rel="noreferrer" title="Sumar otra computadora como worker">
                        <LinkIcon /><span>Conectar esta PC</span><span className={styles.navExt}>↗</span>
                    </a>
                    <button className={styles.navItem} onClick={() => { go('Monitor'); setTimeout(() => document.getElementById('compartir')?.scrollIntoView({ behavior: 'smooth' }), 50) }} title="URL de la red y túnel a internet">
                        <ShareIcon /><span>Compartir</span>
                    </button>
                </nav>

                <div className={styles.sideFoot}>
                    <div className={`${styles.live} ${connected ? styles.liveOk : styles.liveErr}`}>
                        <span className={styles.liveDot} />{connected ? 'En vivo' : 'Reconectando…'}
                    </div>
                    <div className={styles.sideStat}>
                        <b>{workers.length}</b> nodo{workers.length === 1 ? '' : 's'} · <b>{busyNodes}</b> ocupado{busyNodes === 1 ? '' : 's'}
                    </div>
                    <button className={styles.themeBtn} onClick={toggle} title={theme === 'dark' ? 'Cambiar a tema claro' : 'Cambiar a tema oscuro'}>
                        {theme === 'dark' ? <SunIcon /> : <MoonIcon />}
                        <span>{theme === 'dark' ? 'Tema claro' : 'Tema oscuro'}</span>
                    </button>
                </div>
            </aside>

            {/* ── Contenido ── */}
            <main className={styles.main}>
                <header className={styles.pageHeader}>
                    <div>
                        <h1 className={styles.pageTitle}>{tab}</h1>
                        <p className={styles.pageSub}>
                            {tab === 'Casos' && 'Un caso es un conjunto de archivos que entra como una sola solicitud y cierra con un reporte.'}
                            {tab === 'Monitor' && 'Estado del sistema: nodos y su rendimiento, colas por pool y sub-tareas en curso.'}
                            {tab === 'Historial' && 'Todas las sub-tareas, también las terminadas, con su resultado.'}
                        </p>
                    </div>
                    {tab === 'Casos' && !showNew && (
                        <button className="btn btn-primary" onClick={() => setShowNew(true)}>+ Nuevo caso</button>
                    )}
                    {tab === 'Monitor' && (
                        <button className="btn" onClick={refresh} title="Limpiar la vista de sub-tareas en curso">↻ Limpiar</button>
                    )}
                </header>

                {tab === 'Casos' && (
                    <>
                        {showNew && (
                            <SubmitCasePanel
                                onClose={() => setShowNew(false)}
                                onCreated={(id) => { setShowNew(false); casesState.openCase(id); casesState.refresh() }}
                            />
                        )}
                        {casesState.selected && (
                            <Section title="Detalle del caso">
                                <CaseDetail c={casesState.selected} report={casesState.report}
                                    onCancel={casesState.cancel} onClose={casesState.closeCase} />
                            </Section>
                        )}
                        <Section title="Casos" count={`${casesState.cases.length} en total · clic en uno para ver sus sub-tareas y su reporte`}>
                            <CasesPanel cases={casesState.cases} error={casesState.error}
                                selectedId={casesState.selected?.id} onOpen={casesState.openCase} />
                        </Section>
                    </>
                )}

                {tab === 'Monitor' && (
                    <>
                        <Section title="Resumen" count="sub-tareas en todo el sistema">
                            <StatsBar stats={stats} />
                        </Section>

                        <Section title="Nodos y rendimiento" count={`${workers.length} conectado${workers.length === 1 ? '' : 's'}`}>
                            {workers.length === 0 ? (
                                <p className={`card ${styles.empty}`}>
                                    No hay nodos conectados. Abra <a href="/connect" target="_blank" rel="noreferrer">Conectar esta PC</a> en cualquier máquina de la red.
                                </p>
                            ) : (
                                <div className={styles.nodeGrid}>
                                    {workers.map(w => <NodeCard key={w.id} worker={w} history={history} />)}
                                </div>
                            )}
                        </Section>

                        <div className={styles.twoCol}>
                            <Section title="Colas por pool" count="sub-tareas esperando un worker de ese pool">
                                <QueueDepth queue_depth={queue_depth} />
                            </Section>
                            <Section title="Compartir este coordinador" id="compartir" count="para que otra PC se sume como worker">
                                <SharePanel />
                            </Section>
                        </div>

                        <Section title="Casos activos" count={`${by_case.length} abierto${by_case.length === 1 ? '' : 's'} · clic en uno para abrirlo`}>
                            <ActiveCases byCase={by_case} onOpen={(id) => { casesState.openCase(id); go('Casos') }} />
                        </Section>

                        <Section title="Sub-tareas en curso" count={`${jobs.length} activa${jobs.length === 1 ? '' : 's'}`}>
                            <JobTable jobs={jobs} />
                        </Section>
                    </>
                )}

                {tab === 'Historial' && (
                    <Section title="Sub-tareas">
                        <JobHistory />
                    </Section>
                )}
            </main>
        </div>
    )
}

// ── Iconos (SVG inline, 16 px, color del texto) ──
const I = ({ children }) => <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">{children}</svg>
function CasesIcon() { return <I><path d="M3 7h18M3 12h18M3 17h12" /></I> }
function MonitorIcon() { return <I><rect x="3" y="4" width="18" height="12" rx="2" /><path d="M8 20h8M12 16v4" /></I> }
function HistoryIcon() { return <I><circle cx="12" cy="12" r="9" /><path d="M12 7v5l3 2" /></I> }
function LinkIcon() { return <I><path d="M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1" /><path d="M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1" /></I> }
function ShareIcon() { return <I><circle cx="18" cy="5" r="3" /><circle cx="6" cy="12" r="3" /><circle cx="18" cy="19" r="3" /><path d="M8.6 13.5l6.8 4M15.4 6.5l-6.8 4" /></I> }
function SunIcon() { return <I><circle cx="12" cy="12" r="4" /><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" /></I> }
function MoonIcon() { return <I><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z" /></I> }
