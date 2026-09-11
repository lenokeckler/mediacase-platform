package coordinator

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/lenokeckler/mediacase-platform/internal/models"
	"golang.org/x/net/websocket"
)

// El worker abre un WebSocket hacia el coordinador y lo mantiene vivo; el coordinador
// le envía las asignaciones por ese canal. Así el worker nunca necesita un puerto
// abierto ni una IP alcanzable: funciona detrás de cualquier router o firewall.

var (
	ErrNotConnected  = errors.New("worker sin canal abierto")
	ErrWorkerBusy    = errors.New("worker rechazó la sub-tarea (pool lleno)")
	ErrAssignTimeout = errors.New("worker no respondió a la asignación")
)

const assignTimeout = 5 * time.Second

// wsMsg es el mensaje que viaja en ambas direcciones por el canal.
type wsMsg struct {
	Type   string      `json:"type"`             // "assign" | "accept" | "reject" | "ping" | "pong"
	Job    *models.Job `json:"job,omitempty"`    // en "assign"
	JobID  string      `json:"job_id,omitempty"` // en "accept" / "reject"
	Reason string      `json:"reason,omitempty"` // en "reject"
}

type workerConn struct {
	conn    *websocket.Conn
	sendMu  sync.Mutex            // un escritor a la vez sobre el socket
	pendMu  sync.Mutex            // protege pending
	pending map[string]chan wsMsg // job_id → respuesta accept/reject
}

// WorkerHub mantiene el canal abierto de cada worker conectado.
type WorkerHub struct {
	mu    sync.RWMutex
	conns map[string]*workerConn
}

func NewWorkerHub() *WorkerHub {
	return &WorkerHub{conns: make(map[string]*workerConn)}
}

// ServeStream atiende GET /workers/{id}/stream. El worker se conecta y se queda; cada
// mensaje que manda se despacha a quien esté esperando la respuesta de esa sub-tarea.
func (h *WorkerHub) ServeStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "falta id de worker", http.StatusBadRequest)
		return
	}
	websocket.Handler(func(c *websocket.Conn) {
		wc := &workerConn{conn: c, pending: make(map[string]chan wsMsg)}

		h.mu.Lock()
		if old, ok := h.conns[id]; ok {
			old.conn.Close() // reconexión: cerrar el canal viejo
		}
		h.conns[id] = wc
		h.mu.Unlock()
		log.Printf("[whub] worker %s conectó desde %s", id, r.RemoteAddr)

		for {
			var m wsMsg
			if err := websocket.JSON.Receive(c, &m); err != nil {
				break
			}
			switch m.Type {
			case "accept", "reject":
				wc.pendMu.Lock()
				ch, ok := wc.pending[m.JobID]
				wc.pendMu.Unlock()
				if ok {
					ch <- m
				}
			case "ping":
				wc.send(wsMsg{Type: "pong"})
			}
		}

		h.mu.Lock()
		if h.conns[id] == wc {
			delete(h.conns, id)
		}
		h.mu.Unlock()
		log.Printf("[whub] worker %s desconectó", id)
	}).ServeHTTP(w, r)
}

func (wc *workerConn) send(m wsMsg) error {
	wc.sendMu.Lock()
	defer wc.sendMu.Unlock()
	wc.conn.SetWriteDeadline(time.Now().Add(assignTimeout))
	return websocket.JSON.Send(wc.conn, m)
}

// IsConnected indica si el worker tiene un canal abierto ahora mismo.
func (h *WorkerHub) IsConnected(workerID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.conns[workerID]
	return ok
}

// Connected devuelve los IDs de los workers con canal abierto.
func (h *WorkerHub) Connected() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	ids := make([]string, 0, len(h.conns))
	for id := range h.conns {
		ids = append(ids, id)
	}
	return ids
}

// Assign envía la sub-tarea al worker y espera su accept/reject.
// ErrWorkerBusy equivale al antiguo HTTP 429: re-encolar sin contar reintento.
func (h *WorkerHub) Assign(ctx context.Context, workerID string, job *models.Job) error {
	h.mu.RLock()
	wc, ok := h.conns[workerID]
	h.mu.RUnlock()
	if !ok {
		return ErrNotConnected
	}

	reply := make(chan wsMsg, 1)
	wc.pendMu.Lock()
	wc.pending[job.ID] = reply
	wc.pendMu.Unlock()
	defer func() {
		wc.pendMu.Lock()
		delete(wc.pending, job.ID)
		wc.pendMu.Unlock()
	}()

	if err := wc.send(wsMsg{Type: "assign", Job: job}); err != nil {
		return err
	}
	select {
	case m := <-reply:
		if m.Type == "reject" {
			return ErrWorkerBusy
		}
		return nil
	case <-time.After(assignTimeout):
		return ErrAssignTimeout
	case <-ctx.Done():
		return ctx.Err()
	}
}
