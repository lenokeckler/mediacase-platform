package coordinator

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

type QueueDepthSnapshot struct {
	High   int            `json:"high"`
	Normal int            `json:"normal"`
	Low    int            `json:"low"`
	ByPool map[string]int `json:"by_pool"`
}

type SystemSnapshot struct {
	Workers    interface{}        `json:"workers"`
	Jobs       interface{}        `json:"jobs"`
	QueueDepth QueueDepthSnapshot `json:"queue_depth"`
	Stats      interface{}        `json:"stats"`
	ByCase     interface{}        `json:"by_case"`
}

type Hub struct {
	mu      sync.Mutex
	clients map[*websocket.Conn]struct{}
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[*websocket.Conn]struct{}),
	}
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	websocket.Handler(func(conn *websocket.Conn) {
		h.mu.Lock()
		h.clients[conn] = struct{}{}
		h.mu.Unlock()

		log.Printf("[ws] client connected, total: %d", h.clientCount())

		buf := make([]byte, 128)
		for {
			_, err := conn.Read(buf)
			if err != nil {
				break
			}
		}

		h.mu.Lock()
		delete(h.clients, conn)
		h.mu.Unlock()
		log.Printf("[ws] client disconnected, total: %d", h.clientCount())
	}).ServeHTTP(w, r)
}

func (h *Hub) Broadcast(snap SystemSnapshot) {
	data, err := json.Marshal(snap)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for conn := range h.clients {
		conn.Write(data)
	}
}

func (h *Hub) clientCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

func (h *Hub) StartBroadcastLoop(snapshotFn func() SystemSnapshot) {
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			h.Broadcast(snapshotFn())
		}
	}()
}
