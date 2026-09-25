package coordinator

import (
	"testing"

	"github.com/lenokeckler/mediacase-platform/internal/models"
)

// Solo la lógica en memoria (registerNoDB); la persistencia se prueba en integración.
func TestRegister_DetectaProcesoNuevo(t *testing.T) {
	r := &Registry{workers: make(map[string]*models.WorkerInfo)}

	if r.registerNoDB(&models.WorkerInfo{ID: "w1", Instance: "aaa"}) {
		t.Fatal("el primer registro no es un reinicio")
	}
	if r.registerNoDB(&models.WorkerInfo{ID: "w1", Instance: "aaa"}) {
		t.Fatal("el mismo proceso re-registrándose (coordinador reiniciado) NO es un reinicio del worker")
	}
	if !r.registerNoDB(&models.WorkerInfo{ID: "w1", Instance: "bbb"}) {
		t.Fatal("otra instancia con el mismo ID SÍ es un proceso nuevo: hay que reclamar sus jobs")
	}
	if r.registerNoDB(&models.WorkerInfo{ID: "w1", Instance: ""}) {
		t.Fatal("un worker viejo sin instance no debe disparar reclaims espurios")
	}
	if r.registerNoDB(&models.WorkerInfo{ID: "w2", Instance: "ccc"}) {
		t.Fatal("un ID distinto nunca es reinicio")
	}
}

func TestLeastLoadedFor_FiltraPorPool(t *testing.T) {
	r := &Registry{workers: make(map[string]*models.WorkerInfo)}
	r.registerNoDB(&models.WorkerInfo{ID: "v1", Instance: "a", Capabilities: []string{"video"}})
	r.registerNoDB(&models.WorkerInfo{ID: "a1", Instance: "b", Capabilities: []string{"audio"}})
	r.registerNoDB(&models.WorkerInfo{ID: "g1", Instance: "c", Capabilities: []string{"video", "audio", "metadata"}})
	r.workers["v1"].ActiveJobs = 0
	r.workers["a1"].ActiveJobs = 0
	r.workers["g1"].ActiveJobs = 3 // el genérico está cargado

	if w := r.LeastLoadedFor("video"); w == nil || w.ID != "v1" {
		t.Fatalf("video → v1 (menos cargado que g1), fue %v", w)
	}
	if w := r.LeastLoadedFor("audio"); w == nil || w.ID != "a1" {
		t.Fatalf("audio → a1, fue %v", w)
	}
	if w := r.LeastLoadedFor("metadata"); w == nil || w.ID != "g1" {
		t.Fatalf("metadata → solo g1 lo atiende, fue %v", w)
	}
	r.workers["g1"].ActiveJobs = 0
	r.workers["v1"].ActiveJobs = 2
	if w := r.LeastLoadedFor("video"); w == nil || w.ID != "g1" {
		t.Fatalf("video con v1 cargado → g1 (genérico libre), fue %v", w)
	}
	if w := r.LeastLoadedFor("inexistente"); w != nil {
		t.Fatalf("pool sin workers → nil, fue %v", w)
	}
	// Worker viejo sin capabilities declaradas cuenta como genérico
	r.registerNoDB(&models.WorkerInfo{ID: "old", Instance: "d"})
	r.workers["old"].ActiveJobs = 0
	r.workers["g1"].ActiveJobs = 5
	if w := r.LeastLoadedFor("metadata"); w == nil || w.ID != "old" {
		t.Fatalf("worker sin capabilities debe atender cualquier pool, fue %v", w)
	}
}

func f(v float64) *float64 { return &v }

func TestPickFor_AfinidadPrimeroLuegoAyuda(t *testing.T) {
	r := &Registry{workers: make(map[string]*models.WorkerInfo)}
	r.registerNoDB(&models.WorkerInfo{ID: "v1", Capabilities: []string{"video"}, ActiveJobs: 0})
	r.registerNoDB(&models.WorkerInfo{ID: "a1", Capabilities: []string{"audio"}, ActiveJobs: 0})
	all := func(string) bool { return true }

	// Hay worker de audio libre: afinidad.
	w, how := r.PickFor("audio", false, all)
	if w == nil || w.ID != "a1" || how != AssignAffinity {
		t.Fatalf("got %v %q", w, how)
	}
	// El de audio está ocupado y el de video libre: ayuda (work stealing).
	r.workers["a1"].ActiveJobs = 2
	w, how = r.PickFor("audio", false, all)
	if w == nil || w.ID != "v1" || how != AssignHelp {
		t.Fatalf("got %v %q; el nodo libre debe ayudar aunque no sea su pool", w, how)
	}
	// En modo estricto nunca ayuda: vuelve al de audio aunque esté ocupado.
	w, how = r.PickFor("audio", true, all)
	if w == nil || w.ID != "a1" || how != AssignAffinity {
		t.Fatalf("estricto: got %v %q", w, how)
	}
	// Sin nadie con el pool y estricto: nil.
	if w, _ := r.PickFor("metadata", true, all); w != nil {
		t.Fatalf("estricto sin pool: got %v", w)
	}
	// Sin estricto, metadata lo toma el menos cargado de todos.
	if w, how := r.PickFor("metadata", false, all); w == nil || w.ID != "v1" || how != AssignHelp {
		t.Fatalf("ayuda a metadata: got %v %q", w, how)
	}
}

func TestPickFor_EvitaNodosSaturadosYRespetaConexion(t *testing.T) {
	r := &Registry{workers: make(map[string]*models.WorkerInfo)}
	r.registerNoDB(&models.WorkerInfo{ID: "v1", Capabilities: []string{"video"}, ActiveJobs: 0,
		Metrics: &models.NodeMetrics{CPUPercent: 40, MemPercent: 94}}) // RAM al 94 %: saturado
	r.registerNoDB(&models.WorkerInfo{ID: "v2", Capabilities: []string{"video"}, ActiveJobs: 1,
		Metrics: &models.NodeMetrics{CPUPercent: 30, MemPercent: 50}})
	all := func(string) bool { return true }
	// v1 tiene menos activas pero está saturado: gana v2.
	if w, _ := r.PickFor("video", false, all); w == nil || w.ID != "v2" {
		t.Fatalf("got %v; el nodo con RAM > 90 %% va de último", w)
	}
	// Si todos están saturados, igual se asigna (no se deja la cola parada).
	r.workers["v2"].Metrics.MemPercent = 95
	if w, _ := r.PickFor("video", false, all); w == nil {
		t.Fatal("con todos saturados se asigna al menos cargado igual")
	}
	// Un worker sin canal abierto no cuenta.
	onlyV1 := func(id string) bool { return id == "v1" }
	if w, _ := r.PickFor("video", false, onlyV1); w == nil || w.ID != "v1" {
		t.Fatalf("got %v; solo v1 tiene canal", w)
	}
}

func TestPickFor_RepartoProporcionalALaCapacidad(t *testing.T) {
	r := &Registry{workers: make(map[string]*models.WorkerInfo)}
	r.registerNoDB(&models.WorkerInfo{ID: "chica", Capacity: 2})
	r.registerNoDB(&models.WorkerInfo{ID: "potente", Capacity: 8})
	all := func(string) bool { return true }

	// Ráfaga de 10 sub-tareas genéricas: se cuentan al asignar (sin esperar el heartbeat).
	got := map[string]int{}
	for i := 0; i < 10; i++ {
		w, _ := r.PickFor("video", false, all)
		if w == nil {
			t.Fatalf("sub-tarea %d sin worker; quedaban cupos", i)
		}
		got[w.ID]++
		r.NoteAssigned(w.ID)
	}
	if got["potente"] != 8 || got["chica"] != 2 {
		t.Fatalf("reparto %v; quería potente=8 chica=2 (proporcional a la capacidad)", got)
	}
	// Las dos llenas: nadie (la sub-tarea espera en la cola en vez de ser rechazada).
	if w, _ := r.PickFor("video", false, all); w != nil {
		t.Fatalf("con todos llenos debía devolver nil, devolvió %s", w.ID)
	}
}

func TestPickFor_AfinidadHastaLaMitadAntesDePedirAyuda(t *testing.T) {
	r := &Registry{workers: make(map[string]*models.WorkerInfo)}
	r.registerNoDB(&models.WorkerInfo{ID: "video", Capabilities: []string{"video"}, Capacity: 6})
	r.registerNoDB(&models.WorkerInfo{ID: "audio", Capabilities: []string{"audio"}, Capacity: 2})
	all := func(string) bool { return true }

	for i := 0; i < 3; i++ { // 0/6, 1/6, 2/6: por debajo de la mitad se queda en su pool
		w, how := r.PickFor("video", false, all)
		if w.ID != "video" || how != AssignAffinity {
			t.Fatalf("paso %d: %s %s; debía seguir por afinidad", i, w.ID, how)
		}
		r.NoteAssigned(w.ID)
	}
	// 3/6 = la mitad: el de audio (0/2) ayuda.
	if w, how := r.PickFor("video", false, all); w.ID != "audio" || how != AssignHelp {
		t.Fatalf("a la mitad: %s %s; debía ayudar el nodo libre", w.ID, how)
	}
}

func TestPickFor_WorkerViejoSinCapacidad(t *testing.T) {
	r := &Registry{workers: make(map[string]*models.WorkerInfo)}
	r.registerNoDB(&models.WorkerInfo{ID: "viejo", ActiveJobs: 5}) // no informa capacidad
	if w, _ := r.PickFor("video", false, func(string) bool { return true }); w == nil {
		t.Fatal("a un worker sin capacidad declarada no se le salta aunque parezca lleno")
	}
}
