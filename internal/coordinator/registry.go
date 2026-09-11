package coordinator

import (
	"database/sql"
	"log"
	"sync"
	"time"

	"github.com/lenokeckler/mediacase-platform/internal/models"
)

const heartbeatTimeout = 15 * time.Second

type Registry struct {
	mu      sync.RWMutex
	workers map[string]*models.WorkerInfo
	db      *sql.DB
}

func NewRegistry(db *sql.DB) *Registry {
	r := &Registry{
		workers: make(map[string]*models.WorkerInfo),
		db:      db,
	}
	r.loadFromDB() // recupera workers al reiniciar
	return r
}

// loadFromDB recupera workers registrados recientemente al arrancar.
func (r *Registry) loadFromDB() {
	rows, err := r.db.Query(`
		SELECT id, hostname FROM worker_registry
		WHERE last_seen > NOW() - INTERVAL '1 minute'`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		w := &models.WorkerInfo{}
		rows.Scan(&w.ID, &w.Hostname)
		w.LastSeen = time.Now()
		w.Status = "idle"
		r.workers[w.ID] = w
		log.Printf("[registry] recovered worker from DB: %s", w.ID)
	}
}

// Register da de alta (o refresca) un worker. Devuelve true si el ID ya existía pero
// con OTRA instancia: es un proceso nuevo, y los jobs del proceso anterior quedaron huérfanos.
func (r *Registry) Register(w *models.WorkerInfo) (restarted bool) {
	restarted = r.registerNoDB(w)

	// Persistir en DB para sobrevivir reinicios (fuera del lock: es I/O)
	r.db.Exec(`
		INSERT INTO worker_registry (id, hostname, last_seen)
		VALUES ($1, $2, NOW())
		ON CONFLICT (id) DO UPDATE SET hostname=$2, last_seen=NOW()`,
		w.ID, w.Hostname,
	)
	return restarted
}

// registerNoDB es la parte en memoria de Register (probable sin base de datos).
func (r *Registry) registerNoDB(w *models.WorkerInfo) (restarted bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if prev, ok := r.workers[w.ID]; ok && prev.Instance != "" && w.Instance != "" && prev.Instance != w.Instance {
		restarted = true
	}
	w.LastSeen = time.Now()
	w.Status = "idle"
	r.workers[w.ID] = w
	return restarted
}

func (r *Registry) Heartbeat(id string, cpu, mem float64, activeJobs int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.workers[id]
	if !ok {
		return false
	}
	w.LastSeen = time.Now()
	w.CPUPercent = cpu
	w.MemPercent = mem
	w.ActiveJobs = activeJobs
	if activeJobs == 0 {
		w.Status = "idle"
	} else {
		w.Status = "busy"
	}

	// Actualizar timestamp en DB
	r.db.Exec(`
		UPDATE worker_registry SET last_seen=NOW() WHERE id=$1`, id)
	return true
}

func (r *Registry) LeastLoaded() *models.WorkerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var best *models.WorkerInfo
	for _, w := range r.workers {
		if !r.isAlive(w) {
			continue
		}
		if best == nil {
			best = w
			continue
		}
		if w.ActiveJobs < best.ActiveJobs {
			best = w
		} else if w.ActiveJobs == best.ActiveJobs && w.CPUPercent < best.CPUPercent {
			best = w
		}
	}
	return best
}

func (r *Registry) All() []*models.WorkerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]*models.WorkerInfo, 0, len(r.workers))
	for _, w := range r.workers {
		cp := *w
		list = append(list, &cp)
	}
	return list
}

func (r *Registry) EvictStale() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var evicted []string
	for id, w := range r.workers {
		if !r.isAlive(w) {
			evicted = append(evicted, id)
			delete(r.workers, id)
		}
	}
	return evicted
}

func (r *Registry) isAlive(w *models.WorkerInfo) bool {
	return time.Since(w.LastSeen) < heartbeatTimeout
}
