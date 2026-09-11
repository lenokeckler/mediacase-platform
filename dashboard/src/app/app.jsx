import { useState } from 'react'
import { useSystemState } from '../hooks/useSystemState'
import WorkerCard from '../components/WorkerCard'
import JobTable from '../components/JobTable'
import StatsBar from '../components/StatsBar'
import QueueDepth from '../components/QueueDepth'
import SubmitJobPanel from '../components/SubmitJobPanel'
import BatchPanel from '../components/BatchPanel'
import JobHistory from '../components/JobHistory'
import styles from './app.module.css'

const TABS = ['Monitor', 'Submit', 'History']

function ConnectionBadge({ connected }) {
    return (
        <div className={`${styles.connBadge} ${connected ? styles.connOk : styles.connErr}`}>
            <span className={styles.connDot} />
            {connected ? 'Live' : 'Reconnecting…'}
        </div>
    )
}

export default function App() {
    const { workers, jobs, stats, queue_depth, connected, refresh } = useSystemState()
    const [tab, setTab] = useState('Monitor')

    return (
        <div className={styles.layout}>
            {/* ── Header ── */}
            <header className={styles.header}>
                <div className={styles.headerLeft}>
                    <span className={styles.logo}>⚡ MediaCase</span>
                    <span className={styles.subtitle}>Distributed Processing Dashboard</span>
                </div>
                <nav className={styles.tabs}>
                    {TABS.map(t => (
                        <button
                            key={t}
                            className={`${styles.tab} ${tab === t ? styles.tabActive : ''}`}
                            onClick={() => setTab(t)}
                        >
                            {t}
                        </button>
                    ))}
                </nav>
                <ConnectionBadge connected={connected} />
            </header>

            <main className={styles.main}>

                {/* ── Monitor tab ── */}
                {tab === 'Monitor' && (
                    <>
                        <section className={styles.section}>
                            <StatsBar stats={stats} />
                        </section>

                        <section className={styles.section}>
                            <div className={styles.sectionHeader}>
                                <h2 className={styles.sectionTitle}>Worker Nodes</h2>
                                <span className={styles.sectionCount}>{workers.length} registered</span>
                            </div>
                            <div className={styles.workerGrid}>
                                {workers.length === 0 && (
                                    <p className={styles.empty}>No workers registered yet. Run <code>make up</code> to start workers.</p>
                                )}
                                {workers.map(w => <WorkerCard key={w.id} worker={w} />)}
                                <QueueDepth queue_depth={queue_depth} />
                            </div>
                        </section>

                        <section className={styles.section}>
                            <div className={styles.sectionHeader}>
                                <h2 className={styles.sectionTitle}>Live Jobs</h2>
                                <span className={styles.sectionCount}>{jobs.length} in feed</span>
                                <button className={styles.refreshBtn} onClick={refresh} title="Refresh now">
                                    ↻ Refresh
                                </button>
                            </div>
                            <JobTable jobs={jobs} />
                        </section>
                    </>
                )}

                {/* ── Submit tab ── */}
                {tab === 'Submit' && (
                    <section className={styles.section}>
                        <div className={styles.sectionHeader}>
                            <h2 className={styles.sectionTitle}>Job Submission</h2>
                            <span className={styles.sectionCount}>Manual and batch processing</span>
                        </div>
                        <div className={styles.submitGrid}>
                            <SubmitJobPanel />
                            <BatchPanel />
                        </div>
                    </section>
                )}

                {/* ── History tab ── */}
                {tab === 'History' && (
                    <section className={styles.section}>
                        <div className={styles.sectionHeader}>
                            <h2 className={styles.sectionTitle}>Job History</h2>
                            <span className={styles.sectionCount}>All jobs — click a row to expand details</span>
                        </div>
                        <JobHistory />
                    </section>
                )}

            </main>
        </div>
    )
}
