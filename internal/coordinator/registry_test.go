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
