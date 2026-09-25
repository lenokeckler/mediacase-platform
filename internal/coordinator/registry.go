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
	r.loadFromDB() // recupera workers al reiniciar
	return r
}

// loadFromDB recupera workers registrados recientemente al arrancar.
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
				w.Hardware = &h // el worker que sigue vivo no se re-registra: recuperar lo fijo de aquí
			}
		}
		w.LastSeen = time.Now()
		w.Status = "idle"
		r.workers[w.ID] = w
		log.Printf("[registry] recovered worker from DB: %s", w.ID)
	}
}

// Register da de alta (o refresca) un worker. Devuelve true si el ID ya existía pero
// con OTRA instancia: es un proceso nuevo, y los jobs del proceso anterior quedaron huérfanos.
func (r *Registry) Register(w *models.WorkerInfo) (restarted bool) {
	// La instancia anterior se lee de la BD antes de pisarla: si el coordinador también se
	// reinició, la memoria está vacía y es la única forma de saber que el proceso cambió.
	var persisted string
	r.db.QueryRow(`SELECT instance FROM worker_registry WHERE id=$1`, w.ID).Scan(&persisted)
	restarted = r.registerNoDB(w)
	if !restarted && persisted != "" && w.Instance != "" && persisted != w.Instance {
		restarted = true
	}

	// Persistir en DB para sobrevivir reinicios (fuera del lock: es I/O)
	hw, _ := json.Marshal(w.Hardware) // "null" si el worker no lo manda
	// registered_at solo se fija al insertar: un re-registro no cambia el orden de llegada.
	r.db.Exec(`
		INSERT INTO worker_registry (id, hostname, last_seen, role, capabilities, hardware, registered_at, instance, capacity)
		VALUES ($1, $2, NOW(), $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET hostname=$2, last_seen=NOW(), role=$3, capabilities=$4, hardware=$5, instance=$7, capacity=$8`,
		w.ID, w.Hostname, w.Role, strings.Join(w.Capabilities, ","), string(hw), w.RegisteredAt, w.Instance, w.Capacity,
	)
	return restarted
}

// registerNoDB es la parte en memoria de Register (probable sin base de datos).
func (r *Registry) registerNoDB(w *models.WorkerInfo) (restarted bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	prev, known := r.workers[w.ID]
	if known && prev.Instance != "" && w.Instance != "" && prev.Instance != w.Instance {
		restarted = true
	}
	// El orden de llegada se conserva aunque el worker se reinicie o se re-registre.
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

	// Actualizar timestamp en DB
	r.db.Exec(`
		UPDATE worker_registry SET last_seen=NOW() WHERE id=$1`, id)
	return true
}

// LeastLoaded elige el worker vivo con menos carga, sin filtrar por pool.
func (r *Registry) LeastLoaded() *models.WorkerInfo { return r.LeastLoadedFor("") }

// LeastLoadedFor elige, entre los workers vivos de un pool, el que tiene menos sub-tareas
// activas (empate: menor CPU). nil si no hay ninguno. Es el selector estricto; ver PickFor.
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

// Cómo se asignó una sub-tarea: por afinidad (el pool principal del nodo coincide) o por ayuda
// (un nodo libre de otro pool la tomó para no quedarse de brazos cruzados).
const (
	AssignAffinity = "afinidad"
	AssignHelp     = "ayuda"
)

// Umbrales de saturación: un nodo por encima de esto va de último en la elección (pero no se
// excluye: si todos están saturados, igual se reparte).
const (
	overloadedMemPercent = 90
	overloadedCPUPercent = 95
)

// PickFor elige el worker para una sub-tarea del pool dado.
//
// Orden de preferencia: (1) afinidad —nodos cuyo pool principal coincide—, (2) ayuda —cualquier
// nodo con canal abierto— cuando no hay afín o el afín va por la mitad de su capacidad (o está
// saturado) y el otro está proporcionalmente más libre. Dentro de cada grupo: primero los no
// saturados (RAM < 90 %, CPU < 95 % según sus métricas reales), luego la menor fracción ocupada
// (activas ÷ capacidad, que el worker calcula con sus núcleos y su RAM), a igualdad el nodo de
// más capacidad y por último menos CPU. Los nodos llenos no se consideran. Es la conexión con
// la Unidad 1: la heterogeneidad se usa como preferencia (el video a la máquina potente) y la
// capacidad de cada máquina decide cuánto trabajo recibe, en vez de un número fijo para todas.
//
// strict = true vuelve al modelo de pools puros (solo afinidad). connected dice qué workers
// tienen el canal WebSocket abierto ahora mismo.
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

// helpThreshold: el nodo afín se llena hasta la mitad de su capacidad antes de pedir ayuda.
// Así el pool especializado conserva su trabajo (la afinidad sigue viéndose en la demo) y aun
// así los nodos libres entran antes de que el afín se sature.
const helpThreshold = 0.5

// shouldPreferHelp: el nodo de ayuda está sano y proporcionalmente menos cargado, y el afín
// está saturado, lleno o al menos a la mitad de su capacidad.
func shouldPreferHelp(aff, help *models.WorkerInfo) bool {
	if isOverloaded(help) || loadRatio(help) >= loadRatio(aff) {
		return false
	}
	return isOverloaded(aff) || loadRatio(aff) >= helpThreshold
}

// legacyCapacity es la capacidad que se supone a un worker que no la informa (el ZIP viejo
// traía WORKER_POOL_SIZE=2 fijo).
const legacyCapacity = 2

func capacityOf(w *models.WorkerInfo) int {
	if w.Capacity > 0 {
		return w.Capacity
	}
	return legacyCapacity
}

// loadRatio es la fracción ocupada: 2 activas pesan distinto en un nodo de 8 cupos que en uno de 2.
func loadRatio(w *models.WorkerInfo) float64 {
	return float64(w.ActiveJobs) / float64(capacityOf(w))
}

// isFull: el worker declaró su capacidad y ya la tiene cubierta; mandarle más solo produciría
// un rechazo ("pool lleno") y un reencolado. A un worker viejo (capacidad 0) no se le salta.
func isFull(w *models.WorkerInfo) bool {
	return w.Capacity > 0 && w.ActiveJobs >= w.Capacity
}

// NoteAssigned cuenta en el acto una sub-tarea recién entregada. El heartbeat (cada 1 s) trae
// después el número real; sin esto, en una ráfaga todas irían al mismo worker porque su carga
// no cambia hasta el siguiente latido.
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

// lessLoaded ordena: no saturado antes que saturado, menor fracción ocupada (activas ÷
// capacidad), a igualdad el de más capacidad (el nodo potente primero), y por último menos CPU.
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
	// Orden de llegada (y por id si empatan): un mapa de Go itera al azar y el dashboard, que
	// recibe esta lista cada segundo, movía las tarjetas de lugar.
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
		return true // worker sin rol declarado: genérico
	}
	for _, c := range w.Capabilities {
		if c == pool {
			return true
		}
	}
	return false
}
