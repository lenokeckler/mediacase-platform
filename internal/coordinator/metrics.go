// Métricas Prometheus del coordinador (consigna: "monitoreo de recursos: CPU, memoria,
// carga de trabajo por worker, estado de nodos, sub-tareas activas o en espera").
//
// Los workers remotos no exponen ningún puerto (abren ellos la conexión hacia el coordinador),
// así que Prometheus no puede scrapearlos: el coordinador re-exporta lo que recibe por el
// heartbeat de cada uno. Todo lo demás sale de la base y de Redis en el momento del scrape
// (un Collector, no gauges cacheados: así un worker que se fue deja de aparecer solo).
package coordinator

import (
	"context"
	"database/sql"
	"log"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"

	"github.com/lenokeckler/mediacase-platform/internal/db"
	"github.com/lenokeckler/mediacase-platform/internal/queue"
)

const metricsPrefix = "mediacase_"

var (
	descWorkerCPU = prometheus.NewDesc(metricsPrefix+"worker_cpu_percent",
		"CPU del host de cada worker (0-100), según su último heartbeat", []string{"worker", "role"}, nil)
	descWorkerMem = prometheus.NewDesc(metricsPrefix+"worker_mem_percent",
		"Memoria usada del host de cada worker (0-100), según su último heartbeat", []string{"worker", "role"}, nil)
	descWorkerActive = prometheus.NewDesc(metricsPrefix+"worker_active_jobs",
		"Sub-tareas que el worker está ejecutando ahora", []string{"worker", "role"}, nil)
	descWorkerUp = prometheus.NewDesc(metricsPrefix+"worker_up",
		"1 si el worker está registrado y con heartbeat vivo", []string{"worker", "role"}, nil)
	descQueueDepth = prometheus.NewDesc(metricsPrefix+"queue_depth",
		"Sub-tareas esperando en la cola, por pool y prioridad", []string{"pool", "priority"}, nil)
	descCases = prometheus.NewDesc(metricsPrefix+"cases",
		"Casos por estado", []string{"status"}, nil)
	descJobs = prometheus.NewDesc(metricsPrefix+"jobs",
		"Sub-tareas por estado y pool", []string{"status", "pool"}, nil)
	descActiveCases = prometheus.NewDesc(metricsPrefix+"active_cases",
		"Casos abiertos (queued, processing o retrying)", nil, nil)
)

// CaseDuration es el histograma de duración de los casos (creación → cierre); lo observa el
// coordinador cuando el barrier cierra un caso.
var CaseDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    metricsPrefix + "case_duration_seconds",
	Help:    "Duración de cada caso desde que se creó hasta que el barrier lo cerró",
	Buckets: prometheus.ExponentialBuckets(5, 2, 11), // 5 s … ~1.4 h
}, []string{"status"})

// JobsResolved cuenta sub-tareas resueltas (para calcular throughput con rate()).
var JobsResolved = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: metricsPrefix + "jobs_resolved_total",
	Help: "Sub-tareas que llegaron a completed o failed",
}, []string{"status", "pool"})

// ObserveCaseClosed registra la duración de un caso recién cerrado.
func ObserveCaseClosed(status string, createdAt, closedAt time.Time) {
	if closedAt.IsZero() {
		closedAt = time.Now()
	}
	CaseDuration.WithLabelValues(status).Observe(closedAt.Sub(createdAt).Seconds())
}

// collector lee registry, base y cola en cada scrape.
type collector struct {
	registry *Registry
	queue    *queue.Queue
	db       *sql.DB
}

func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{descWorkerCPU, descWorkerMem, descWorkerActive, descWorkerUp,
		descQueueDepth, descCases, descJobs, descActiveCases} {
		ch <- d
	}
}

func (c *collector) Collect(ch chan<- prometheus.Metric) {
	for _, w := range c.registry.All() {
		role := w.Role
		if role == "" {
			role = "all"
		}
		ch <- prometheus.MustNewConstMetric(descWorkerUp, prometheus.GaugeValue, 1, w.ID, role)
		ch <- prometheus.MustNewConstMetric(descWorkerCPU, prometheus.GaugeValue, w.CPUPercent, w.ID, role)
		ch <- prometheus.MustNewConstMetric(descWorkerMem, prometheus.GaugeValue, w.MemPercent, w.ID, role)
		ch <- prometheus.MustNewConstMetric(descWorkerActive, prometheus.GaugeValue, float64(w.ActiveJobs), w.ID, role)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, pool := range queue.Pools {
		for prio, n := range c.queue.DepthFor(ctx, pool) {
			ch <- prometheus.MustNewConstMetric(descQueueDepth, prometheus.GaugeValue, float64(n), pool, prio)
		}
	}

	if counts, err := db.CountCasesByStatus(c.db); err == nil {
		open := 0
		for st, n := range counts {
			ch <- prometheus.MustNewConstMetric(descCases, prometheus.GaugeValue, float64(n), st)
			if st == "queued" || st == "processing" || st == "retrying" {
				open += n
			}
		}
		ch <- prometheus.MustNewConstMetric(descActiveCases, prometheus.GaugeValue, float64(open))
	} else {
		log.Printf("[metrics] casos por estado: %v", err)
	}
	if cells, err := db.CountJobsByStatusPool(c.db); err == nil {
		for _, cell := range cells {
			pool := cell.Pool
			if pool == "" {
				pool = "none"
			}
			ch <- prometheus.MustNewConstMetric(descJobs, prometheus.GaugeValue, float64(cell.Count), cell.Status, pool)
		}
	} else {
		log.Printf("[metrics] sub-tareas por estado: %v", err)
	}
}

var registerOnce sync.Once

// MetricsHandler registra el collector del coordinador (una sola vez) y devuelve el handler
// de /metrics.
func MetricsHandler(registry *Registry, q *queue.Queue, database *sql.DB) http.Handler {
	registerOnce.Do(func() {
		prometheus.MustRegister(&collector{registry: registry, queue: q, db: database})
	})
	return promhttp.Handler()
}
