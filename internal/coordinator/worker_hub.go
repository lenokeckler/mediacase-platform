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

var (
	ErrNotConnected  = errors.New("worker sin canal abierto")
	ErrWorkerBusy    = errors.New("worker rechazó la sub-tarea (pool lleno)")
	ErrAssignTimeout = errors.New("worker no respondió a la asignación")
)

const assignTimeout = 5 * time.Second

type wsMsg struct {
	Type   string      `json:"type"`
	Job    *models.Job `json:"job,omitempty"`
	JobID  string      `json:"job_id,omitempty"`
	Reason string      `json:"reason,omitempty"`
}

type workerConn struct {
	conn    *websocket.Conn
	sendMu  sync.Mutex
	pendMu  sync.Mutex
	pending map[string]chan wsMsg
}

type WorkerHub struct {
	mu    sync.RWMutex
	conns map[string]*workerConn
}

func NewWorkerHub() *WorkerHub {
	return &WorkerHub{conns: make(map[string]*workerConn)}
}

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
			old.conn.Close()
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

func (h *WorkerHub) IsConnected(workerID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.conns[workerID]
	return ok
}

func (h *WorkerHub) Connected() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	ids := make([]string, 0, len(h.conns))
	for id := range h.conns {
		ids = append(ids, id)
	}
	return ids
}

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
