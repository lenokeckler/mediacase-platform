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

var ErrNoMessages = errors.New("no messages available")

var Pools = []string{"video", "audio", "metadata"}

const (
	GroupName = "workers"
	DLQ       = "jobs:failed"

	readBlock = 500 * time.Millisecond
)

var priorityNames = []string{"high", "normal", "low"}

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

func (q *Queue) StreamLen(ctx context.Context, stream string) (int64, error) {
	return q.client.XLen(ctx, stream).Result()
}

type Depth struct {
	ByPool     map[string]int64
	ByPriority map[string]int64
	Total      int64
}

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

func (q *Queue) DepthFor(ctx context.Context, pool string) map[string]int64 {
	out := make(map[string]int64, len(priorityNames))
	for _, n := range priorityNames {
		out[n] = q.waiting(ctx, "jobs:"+pool+":"+n)
	}
	return out
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

		after, err := q.client.XRange(ctx, stream, "("+g.LastDeliveredID, "+").Result()
		if err != nil {
			return g.Pending
		}
		return int64(len(after)) + g.Pending
	}
	return 0
}

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

func (q *Queue) Ack(ctx context.Context, stream, msgID string) error {
	if err := q.client.XAck(ctx, stream, GroupName, msgID).Err(); err != nil {
		return err
	}
	return q.client.XDel(ctx, stream, msgID).Err()
}

func (q *Queue) EnsureGroups(ctx context.Context) {
	for _, stream := range AllStreams() {
		q.client.XGroupCreateMkStream(ctx, stream, GroupName, "0")
	}
}
