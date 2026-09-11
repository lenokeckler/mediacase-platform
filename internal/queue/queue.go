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

// ErrNoMessages is returned when the queue is empty.
var ErrNoMessages = errors.New("no messages available")

// StreamForPriority is the exported version so the scheduler can call Ack.
func StreamForPriority(p int) string {
	return streamForPriority(p)
}

const (
	StreamHigh   = "jobs:high"
	StreamNormal = "jobs:normal"
	StreamLow    = "jobs:low"
	GroupName    = "workers"
	DLQ          = "jobs:failed"
)

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
// Used by the coordinator to report queue depth to the dashboard.
func (q *Queue) StreamLen(ctx context.Context, stream string) (int64, error) {
	return q.client.XLen(ctx, stream).Result()
}

// Enqueue adds a job to the appropriate priority stream
func (q *Queue) Enqueue(ctx context.Context, job *models.Job) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal job: %w", err)
	}
	stream := streamForPriority(job.Priority)
	return q.client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{"payload": string(data)},
	}).Err()
}

// Dequeue blocks and reads the next job for a given consumer
func (q *Queue) Dequeue(ctx context.Context, consumerID string) (*models.Job, string, error) {
	streams := []string{StreamHigh, StreamNormal, StreamLow, ">", ">", ">"}
	res, err := q.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    GroupName,
		Consumer: consumerID,
		Streams:  streams,
		Count:    1,
		Block:    5 * time.Second,
	}).Result()
	if err != nil {
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

// Ack acknowledges a processed message
func (q *Queue) Ack(ctx context.Context, stream, msgID string) error {
	return q.client.XAck(ctx, stream, GroupName, msgID).Err()
}

// EnsureGroups creates consumer groups if they don't exist
func (q *Queue) EnsureGroups(ctx context.Context) {
	for _, stream := range []string{StreamHigh, StreamNormal, StreamLow} {
		q.client.XGroupCreateMkStream(ctx, stream, GroupName, "0")
	}
}

func streamForPriority(p int) string {
	if p >= 8 {
		return StreamHigh
	} else if p >= 4 {
		return StreamNormal
	}
	return StreamLow
}
