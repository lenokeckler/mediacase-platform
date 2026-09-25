package coordinator

import (
	"database/sql"
	"encoding/json"
	"log"
	"sort"
	"strings"
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
	r.loadFromDB()
	return r
}

func (r *Registry) loadFromDB() {
	rows, err := r.db.Query(`
		SELECT id, hostname, COALESCE(role,''), COALESCE(capabilities,''), COALESCE(hardware::text,'null'),
		       COALESCE(registered_at, last_seen), instance, capacity
		FROM worker_registry WHERE last_seen > NOW() - INTERVAL '1 minute'`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		w := &models.WorkerInfo{}
		var caps, hw string
		rows.Scan(&w.ID, &w.Hostname, &w.Role, &caps, &hw, &w.RegisteredAt, &w.Instance, &w.Capacity)
		if caps != "" {
			w.Capabilities = strings.Split(caps, ",")
		}
		if hw != "" && hw != "null" {
			var h models.Hardware
			if json.Unmarshal([]byte(hw), &h) == nil {
				w.Hardware = &h
			}
		}
		w.LastSeen = time.Now()
		w.Status = "idle"
		r.workers[w.ID] = w
		log.Printf("[registry] recovered worker from DB: %s", w.ID)
	}
}

func (r *Registry) Register(w *models.WorkerInfo) (restarted bool) {

	var persisted string
	r.db.QueryRow(`SELECT instance FROM worker_registry WHERE id=$1`, w.ID).Scan(&persisted)
	restarted = r.registerNoDB(w)
	if !restarted && persisted != "" && w.Instance != "" && persisted != w.Instance {
		restarted = true
	}

	hw, _ := json.Marshal(w.Hardware)

	r.db.Exec(`
		INSERT INTO worker_registry (id, hostname, last_seen, role, capabilities, hardware, registered_at, instance, capacity)
		VALUES ($1, $2, NOW(), $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET hostname=$2, last_seen=NOW(), role=$3, capabilities=$4, hardware=$5, instance=$7, capacity=$8`,
		w.ID, w.Hostname, w.Role, strings.Join(w.Capabilities, ","), string(hw), w.RegisteredAt, w.Instance, w.Capacity,
	)
	return restarted
}

func (r *Registry) registerNoDB(w *models.WorkerInfo) (restarted bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	prev, known := r.workers[w.ID]
	if known && prev.Instance != "" && w.Instance != "" && prev.Instance != w.Instance {
		restarted = true
	}

	if known && !prev.RegisteredAt.IsZero() {
		w.RegisteredAt = prev.RegisteredAt
	} else if w.RegisteredAt.IsZero() {
		w.RegisteredAt = time.Now()
	}
	w.LastSeen = time.Now()
	w.Status = "idle"
	r.workers[w.ID] = w
	return restarted
}

func (r *Registry) Heartbeat(id string, cpu, mem float64, activeJobs int, metrics *models.NodeMetrics) bool {
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
	if metrics != nil {
		w.Metrics = metrics
	}
	if activeJobs == 0 {
		w.Status = "idle"
	} else {
		w.Status = "busy"
	}

	r.db.Exec(`
		UPDATE worker_registry SET last_seen=NOW() WHERE id=$1`, id)
	return true
}

func (r *Registry) LeastLoaded() *models.WorkerInfo { return r.LeastLoadedFor("") }

func (r *Registry) LeastLoadedFor(pool string) *models.WorkerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var best *models.WorkerInfo
	for _, w := range r.workers {
		if !r.isAlive(w) || (pool != "" && !hasCapability(w, pool)) {
			continue
		}
		if best == nil || lessLoaded(w, best) {
			best = w
		}
	}
	return best
}

const (
	AssignAffinity = "afinidad"
	AssignHelp     = "ayuda"
)

const (
	overloadedMemPercent = 90
	overloadedCPUPercent = 95
)

func (r *Registry) PickFor(pool string, strict bool, connected func(id string) bool) (*models.WorkerInfo, string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var bestAff, bestHelp *models.WorkerInfo
	for _, w := range r.workers {
		if !r.isAlive(w) || (connected != nil && !connected(w.ID)) || isFull(w) {
			continue
		}
		if hasCapability(w, pool) {
			if bestAff == nil || lessLoaded(w, bestAff) {
				bestAff = w
			}
		} else if !strict {
			if bestHelp == nil || lessLoaded(w, bestHelp) {
				bestHelp = w
			}
		}
	}
	if bestAff != nil && (strict || bestHelp == nil || !shouldPreferHelp(bestAff, bestHelp)) {
		return bestAff, AssignAffinity
	}
	if bestHelp != nil {
		return bestHelp, AssignHelp
	}
	return nil, ""
}

const helpThreshold = 0.5

func shouldPreferHelp(aff, help *models.WorkerInfo) bool {
	if isOverloaded(help) || loadRatio(help) >= loadRatio(aff) {
		return false
	}
	return isOverloaded(aff) || loadRatio(aff) >= helpThreshold
}

const legacyCapacity = 2

func capacityOf(w *models.WorkerInfo) int {
	if w.Capacity > 0 {
		return w.Capacity
	}
	return legacyCapacity
}

func loadRatio(w *models.WorkerInfo) float64 {
	return float64(w.ActiveJobs) / float64(capacityOf(w))
}

func isFull(w *models.WorkerInfo) bool {
	return w.Capacity > 0 && w.ActiveJobs >= w.Capacity
}

func (r *Registry) NoteAssigned(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if w, ok := r.workers[id]; ok {
		w.ActiveJobs++
		w.Status = "busy"
	}
}

func isOverloaded(w *models.WorkerInfo) bool {
	cpu, mem := w.CPUPercent, w.MemPercent
	if w.Metrics != nil {
		cpu, mem = w.Metrics.CPUPercent, w.Metrics.MemPercent
	}
	return mem >= overloadedMemPercent || cpu >= overloadedCPUPercent
}

func lessLoaded(a, b *models.WorkerInfo) bool {
	oa, ob := isOverloaded(a), isOverloaded(b)
	if oa != ob {
		return !oa
	}
	if ra, rb := loadRatio(a), loadRatio(b); ra != rb {
		return ra < rb
	}
	if ca, cb := capacityOf(a), capacityOf(b); ca != cb {
		return ca > cb
	}
	ca, cb := a.CPUPercent, b.CPUPercent
	if a.Metrics != nil {
		ca = a.Metrics.CPUPercent
	}
	if b.Metrics != nil {
		cb = b.Metrics.CPUPercent
	}
	return ca < cb
}

func (r *Registry) Remove(id, instance string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.workers[id]
	if !ok {
		return false
	}
	if instance != "" && w.Instance != "" && w.Instance != instance {
		return false
	}
	delete(r.workers, id)
	return true
}

func (r *Registry) All() []*models.WorkerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]*models.WorkerInfo, 0, len(r.workers))
	for _, w := range r.workers {
		cp := *w
		list = append(list, &cp)
	}

	sort.Slice(list, func(i, j int) bool {
		if !list[i].RegisteredAt.Equal(list[j].RegisteredAt) {
			return list[i].RegisteredAt.Before(list[j].RegisteredAt)
		}
		return list[i].ID < list[j].ID
	})
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

func hasCapability(w *models.WorkerInfo, pool string) bool {
	if len(w.Capabilities) == 0 {
		return true
	}
	for _, c := range w.Capabilities {
		if c == pool {
			return true
		}
	}
	return false
}
