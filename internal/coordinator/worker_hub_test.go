package coordinator

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lenokeckler/mediacase-platform/internal/models"
	"golang.org/x/net/websocket"
)

// fakeWorker conecta al hub y responde a cada "assign" según decide(job).
func fakeWorker(t *testing.T, srvURL, id string, decide func(*models.Job) string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(srvURL, "http") + "/workers/" + id + "/stream"
	c, err := websocket.Dial(wsURL, "", srvURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	go func() {
		for {
			var m wsMsg
			if websocket.JSON.Receive(c, &m) != nil {
				return
			}
			if m.Type == "assign" && m.Job != nil {
				websocket.JSON.Send(c, wsMsg{Type: decide(m.Job), JobID: m.Job.ID})
			}
		}
	}()
	return c
}

func waitConnected(t *testing.T, hub *WorkerHub, id string) {
	t.Helper()
	for i := 0; i < 50; i++ {
		if hub.IsConnected(id) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("worker %s nunca apareció como conectado", id)
}

func newHubServer(t *testing.T) (*WorkerHub, *httptest.Server) {
	t.Helper()
	hub := NewWorkerHub()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /workers/{id}/stream", hub.ServeStream)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return hub, srv
}

func TestWorkerHub_SinCanal(t *testing.T) {
	hub, _ := newHubServer(t)
	err := hub.Assign(context.Background(), "w1", &models.Job{ID: "j0"})
	if !errors.Is(err, ErrNotConnected) {
		t.Fatalf("sin canal: quería ErrNotConnected, fue %v", err)
	}
}

func TestWorkerHub_AcceptYReject(t *testing.T) {
	hub, srv := newHubServer(t)
	c := fakeWorker(t, srv.URL, "w1", func(j *models.Job) string {
		if j.ID == "j2" {
			return "reject"
		}
		return "accept"
	})
	defer c.Close()
	waitConnected(t, hub, "w1")

	if err := hub.Assign(context.Background(), "w1", &models.Job{ID: "j1"}); err != nil {
		t.Fatalf("j1 debía aceptarse: %v", err)
	}
	if err := hub.Assign(context.Background(), "w1", &models.Job{ID: "j2"}); !errors.Is(err, ErrWorkerBusy) {
		t.Fatalf("j2 debía rechazarse con ErrWorkerBusy, fue %v", err)
	}
}

func TestWorkerHub_DesconexionLimpiaElRegistro(t *testing.T) {
	hub, srv := newHubServer(t)
	c := fakeWorker(t, srv.URL, "w1", func(*models.Job) string { return "accept" })
	waitConnected(t, hub, "w1")

	c.Close()
	for i := 0; i < 50 && hub.IsConnected("w1"); i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if hub.IsConnected("w1") {
		t.Fatal("tras cerrar el socket, el worker sigue como conectado")
	}
	if err := hub.Assign(context.Background(), "w1", &models.Job{ID: "j3"}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("tras desconectar: quería ErrNotConnected, fue %v", err)
	}
}

func TestWorkerHub_ReconexionReemplazaCanal(t *testing.T) {
	hub, srv := newHubServer(t)
	old := fakeWorker(t, srv.URL, "w1", func(*models.Job) string { return "reject" })
	waitConnected(t, hub, "w1")

	// El mismo worker vuelve a conectar (p. ej. tras un corte de red): el canal nuevo manda.
	nu := fakeWorker(t, srv.URL, "w1", func(*models.Job) string { return "accept" })
	defer nu.Close()
	time.Sleep(100 * time.Millisecond)

	if err := hub.Assign(context.Background(), "w1", &models.Job{ID: "j4"}); err != nil {
		t.Fatalf("el canal nuevo debía aceptar: %v", err)
	}
	old.Close()
	time.Sleep(50 * time.Millisecond)
	if !hub.IsConnected("w1") {
		t.Fatal("cerrar el canal viejo no debe desregistrar al canal nuevo")
	}
}

func TestWorkerHub_TimeoutSiElWorkerNoResponde(t *testing.T) {
	hub, srv := newHubServer(t)
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/workers/mudo/stream"
	c, err := websocket.Dial(wsURL, "", srv.URL) // conecta pero nunca responde
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	waitConnected(t, hub, "mudo")

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	err = hub.Assign(ctx, "mudo", &models.Job{ID: "j5"})
	if err == nil {
		t.Fatal("un worker que no responde no puede contar como asignación exitosa")
	}
}
