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
