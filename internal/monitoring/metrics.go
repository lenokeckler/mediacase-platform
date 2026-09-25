package monitoring

import (
	"log"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
)

var workerCPUPercent = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "worker_cpu_percent",
	Help: "Uso de CPU del proceso worker (0-100)",
}, []string{"worker_id"})

var workerMemMB = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "worker_memory_mb",
	Help: "Memoria residente del proceso worker en MB",
}, []string{"worker_id"})

var hostMemPercent = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "worker_host_mem_percent",
	Help: "Porcentaje de uso de memoria del host",
}, []string{"worker_id"})

var workerGoroutines = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "worker_goroutines",
	Help: "Número de goroutines activas en el proceso worker",
}, []string{"worker_id"})

var JobsCompleted = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "worker_jobs_completed_total",
	Help: "Total de jobs completados exitosamente",
}, []string{"worker_id", "operation"})

var JobsFailed = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "worker_jobs_failed_total",
	Help: "Total de jobs fallidos",
}, []string{"worker_id", "operation"})

var ActiveJobs = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "worker_active_jobs",
	Help: "Número de jobs que se están procesando ahora mismo",
}, []string{"worker_id"})

var JobDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "worker_job_duration_seconds",
	Help:    "Duración del procesamiento multimedia en segundos",
	Buckets: prometheus.ExponentialBuckets(1, 2, 10),
}, []string{"worker_id", "operation"})

var (
	workerID string
	lastCPU  float64
	lastMem  float64
	statsMu  sync.RWMutex
)

func Init(id string) {
	workerID = id
	go collectLoop()
	log.Printf("[metrics] exportador Prometheus iniciado para worker %s", id)
}

func collectLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	pid := int32(os.Getpid())
	proc, err := process.NewProcess(pid)
	if err != nil {
		log.Printf("[metrics] no se pudo adjuntar al proceso %d: %v", pid, err)
	}

	for range ticker.C {
		var currentCPU, currentMem float64

		if pcts, err := cpu.Percent(0, false); err == nil && len(pcts) > 0 {
			workerCPUPercent.WithLabelValues(workerID).Set(pcts[0])
			currentCPU = pcts[0]
		}

		if proc != nil {
			if mi, err := proc.MemoryInfo(); err == nil {
				workerMemMB.WithLabelValues(workerID).Set(float64(mi.RSS) / 1024 / 1024)
			}
		}

		if vm, err := mem.VirtualMemory(); err == nil {
			hostMemPercent.WithLabelValues(workerID).Set(vm.UsedPercent)
			currentMem = vm.UsedPercent
		}

		workerGoroutines.WithLabelValues(workerID).Set(float64(runtime.NumGoroutine()))

		statsMu.Lock()
		lastCPU = currentCPU
		lastMem = currentMem
		statsMu.Unlock()
	}
}

func GetSystemStats() (cpuPercent, memPercent float64) {
	statsMu.RLock()
	defer statsMu.RUnlock()
	return lastCPU, lastMem
}
