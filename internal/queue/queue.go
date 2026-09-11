package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lenokeckler/mediacase-platform/internal/models"
	"github.com/redis/go-redis/v9"
)

// Cola de sub-tareas sobre Redis Streams.
//
// Hay un stream por (pool, prioridad): jobs:video:high, jobs:audio:normal, ... Es literalmente
// "encolar cada sub-tarea en el mecanismo de distribución adecuado" (consigna §3): el scheduler
// solo saca de la cola de un pool cuando hay un worker de ese pool con capacidad. Dentro de
// cada pool, high se atiende antes que normal y normal antes que low.

// ErrNoMessages is returned when the queue is empty.
var ErrNoMessages = errors.New("no messages available")

// Pools conocidos (deben coincidir con internal/cases.PoolFor y cmd/worker.RoleCapabilities).
var Pools = []string{"video", "audio", "metadata"}

const (
	GroupName = "workers"
	DLQ       = "jobs:failed"

	// Cuánto bloquea una lectura vacía. Corto, porque el scheduler recorre los 3 pools en serie.
	readBlock = 500 * time.Millisecond
)

var priorityNames = []string{"high", "normal", "low"}

// StreamFor devuelve el stream de un pool y una prioridad numérica (1-10).
func StreamFor(pool string, priority int) string {
	return "jobs:" + pool + ":" + priorityName(priority)
}

func priorityName(p int) string {
	switch {
	case p >= 8:
		return "high"
	case p >= 4:
		return "normal"
	default:
		return "low"
	}
}

// AllStreams lista los 9 streams (3 pools × 3 prioridades).
func AllStreams() []string {
	out := make([]string, 0, len(Pools)*len(priorityNames))
	for _, p := range Pools {
		for _, n := range priorityNames {
			out = append(out, "jobs:"+p+":"+n)
		}
	}
	return out
}

type Queue struct {
	client *redis.Client
}

func New(addr, password string) *Queue {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       0,
	})
	return &Queue{client: rdb}
}

func (q *Queue) Ping(ctx context.Context) error {
	return q.client.Ping(ctx).Err()
}

// StreamLen returns the number of messages in a stream (pending + unread).
func (q *Queue) StreamLen(ctx context.Context, stream string) (int64, error) {
	return q.client.XLen(ctx, stream).Result()
}

// Depth resume la profundidad de las colas: por pool y por prioridad (sumando pools).
type Depth struct {
	ByPool     map[string]int64
	ByPriority map[string]int64 // "high" | "normal" | "low"
	Total      int64
}

// Depth cuenta lo que de verdad espera: entradas aún no entregadas al grupo (lag) más las
// entregadas y no confirmadas (pending). XLEN no sirve: cuenta también lo ya procesado.
func (q *Queue) Depth(ctx context.Context) Depth {
	d := Depth{ByPool: map[string]int64{}, ByPriority: map[string]int64{}}
	for _, p := range Pools {
		for _, n := range priorityNames {
			l := q.waiting(ctx, "jobs:"+p+":"+n)
			d.ByPool[p] += l
			d.ByPriority[n] += l
			d.Total += l
		}
	}
	return d
}

func (q *Queue) waiting(ctx context.Context, stream string) int64 {
	groups, err := q.client.XInfoGroups(ctx, stream).Result()
	if err != nil {
		return 0
	}
	for _, g := range groups {
		if g.Name != GroupName {
			continue
		}
		if g.Lag >= 0 {
			return g.Lag + g.Pending
		}
		// Redis pierde la cuenta del lag (lo reporta nil, go-redis -1) cuando se borran entradas
		// del stream con XDEL, que es lo que hace Ack. Se cuenta a mano lo que sigue después del
		// último id entregado; el backlog es de cientos de sub-tareas, no millones.
		after, err := q.client.XRange(ctx, stream, "("+g.LastDeliveredID, "+").Result()
		if err != nil {
			return g.Pending
		}
		return int64(len(after)) + g.Pending
	}
	return 0
}

// Enqueue adds a job to the stream of its pool and priority.
func (q *Queue) Enqueue(ctx context.Context, job *models.Job) error {
	if job.Pool == "" {
		return fmt.Errorf("job %s sin pool: el routing debe correr antes de encolar", job.ID)
	}
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal job: %w", err)
	}
	return q.client.XAdd(ctx, &redis.XAddArgs{
		Stream: StreamFor(job.Pool, job.Priority),
		Values: map[string]any{"payload": string(data)},
	}).Err()
}

// Dequeue lee la siguiente sub-tarea del pool (high → normal → low). Devuelve (nil, "", nil)
// si no hay nada en ese pool tras readBlock.
func (q *Queue) Dequeue(ctx context.Context, consumerID, pool string) (*models.Job, string, error) {
	streams := []string{
		"jobs:" + pool + ":high", "jobs:" + pool + ":normal", "jobs:" + pool + ":low",
		">", ">", ">",
	}
	res, err := q.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    GroupName,
		Consumer: consumerID,
		Streams:  streams,
		Count:    1,
		Block:    readBlock,
	}).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, "", nil
		}
		return nil, "", err
	}
	for _, s := range res {
		for _, msg := range s.Messages {
			payload, ok := msg.Values["payload"].(string)
			if !ok {
				continue
			}
			var job models.Job
			if err := json.Unmarshal([]byte(payload), &job); err != nil {
				continue
			}
			return &job, msg.ID, nil
		}
	}
	return nil, "", nil
}

// Ack confirma el mensaje y lo borra del stream: lo procesado no debe seguir ocupando memoria.
func (q *Queue) Ack(ctx context.Context, stream, msgID string) error {
	if err := q.client.XAck(ctx, stream, GroupName, msgID).Err(); err != nil {
		return err
	}
	return q.client.XDel(ctx, stream, msgID).Err()
}

// EnsureGroups creates the consumer group on every stream if it doesn't exist.
func (q *Queue) EnsureGroups(ctx context.Context) {
	for _, stream := range AllStreams() {
		q.client.XGroupCreateMkStream(ctx, stream, GroupName, "0")
	}
}
