package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Handler represents a task worker function.
type Handler func(ctx context.Context, payload []byte) error

// Queue defines a robust, optional task queue.
type Queue interface {
	Register(taskType string, h Handler)
	Start(ctx context.Context) error
	Enqueue(ctx context.Context, taskType string, payload []byte) error
}

// New returns a Queue implementation. If redisURL is empty, it returns a local
// InMemoryQueue. If redisURL is set, it returns a RedisQueue.
func New(redisURL string) Queue {
	if redisURL == "" {
		log.Println("queue: Redis URL is empty. Using in-memory task queue.")
		return &InMemoryQueue{
			handlers: make(map[string]Handler),
			ch:       make(chan task, 1000),
		}
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Printf("queue: failed to parse redis URL %q: %v. Falling back to in-memory queue.", redisURL, err)
		return &InMemoryQueue{
			handlers: make(map[string]Handler),
			ch:       make(chan task, 1000),
		}
	}

	client := redis.NewClient(opts)
	// Ping to verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("queue: Redis connection failed to %q: %v. Falling back to in-memory queue.", redisURL, err)
		return &InMemoryQueue{
			handlers: make(map[string]Handler),
			ch:       make(chan task, 1000),
		}
	}

	log.Printf("queue: connected to Redis task queue at %s", opts.Addr)
	return &RedisQueue{
		client:   client,
		handlers: make(map[string]Handler),
		key:      "led:queue:tasks",
	}
}

type task struct {
	Type    string `json:"type"`
	Payload []byte `json:"payload"`
}

// --- InMemoryQueue implementation ---

type InMemoryQueue struct {
	handlers map[string]Handler
	ch       chan task
	wg       sync.WaitGroup
}

func (q *InMemoryQueue) Register(taskType string, h Handler) {
	q.handlers[taskType] = h
}

func (q *InMemoryQueue) Start(ctx context.Context) error {
	q.wg.Add(4) // 4 workers
	for i := 0; i < 4; i++ {
		go func() {
			defer q.wg.Done()
			for {
				select {
				case t, ok := <-q.ch:
					if !ok {
						return
					}
					h, exists := q.handlers[t.Type]
					if !exists {
						log.Printf("queue: no handler registered for task type %q", t.Type)
						continue
					}
					if err := h(ctx, t.Payload); err != nil {
						log.Printf("queue: task %q failed: %v", t.Type, err)
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	return nil
}

func (q *InMemoryQueue) Enqueue(ctx context.Context, taskType string, payload []byte) error {
	select {
	case q.ch <- task{Type: taskType, Payload: payload}:
		return nil
	default:
		// Queue full, fallback to instant goroutine execution
		h, exists := q.handlers[taskType]
		if !exists {
			return fmt.Errorf("queue: full and no handler registered for %q", taskType)
		}
		go func() {
			if err := h(context.Background(), payload); err != nil {
				log.Printf("queue: fallback instant execution of %q failed: %v", taskType, err)
			}
		}()
		return nil
	}
}

// --- RedisQueue implementation ---

type RedisQueue struct {
	client   *redis.Client
	handlers map[string]Handler
	key      string
	wg       sync.WaitGroup
}

func (q *RedisQueue) Register(taskType string, h Handler) {
	q.handlers[taskType] = h
}

func (q *RedisQueue) Start(ctx context.Context) error {
	q.wg.Add(4) // 4 workers
	for i := 0; i < 4; i++ {
		go func() {
			defer q.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					// Pop task from list (blocking call with 2s timeout)
					res, err := q.client.BRPop(ctx, 2*time.Second, q.key).Result()
					if err != nil {
						if err != redis.Nil && ctx.Err() == nil {
							log.Printf("queue: redis BRPop error: %v. Sleeping 1s...", err)
							time.Sleep(time.Second)
						}
						continue
					}
					if len(res) < 2 {
						continue
					}
					var t task
					if err := json.Unmarshal([]byte(res[1]), &t); err != nil {
						log.Printf("queue: failed to unmarshal task: %v", err)
						continue
					}
					h, exists := q.handlers[t.Type]
					if !exists {
						log.Printf("queue: no handler registered for task type %q", t.Type)
						continue
					}
					// Process task with retry on transient failures
					if err := h(ctx, t.Payload); err != nil {
						log.Printf("queue: task %q failed: %v. Retrying in 5s...", t.Type, err)
						// Basic retry mechanism by re-enqueueing
						go func(retTask task) {
							time.Sleep(5 * time.Second)
							data, _ := json.Marshal(retTask)
							_ = q.client.LPush(context.Background(), q.key, data).Err()
						}(t)
					}
				}
			}
		}()
	}
	return nil
}

func (q *RedisQueue) Enqueue(ctx context.Context, taskType string, payload []byte) error {
	t := task{Type: taskType, Payload: payload}
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}

	err = q.client.LPush(ctx, q.key, data).Err()
	if err != nil {
		log.Printf("queue: redis LPush failed: %v. Falling back to instant execution.", err)
		// Connection failed - run instantly in background as fallback
		h, exists := q.handlers[taskType]
		if exists {
			go func() {
				if err := h(context.Background(), payload); err != nil {
					log.Printf("queue: fallback instant execution of %q failed: %v", taskType, err)
				}
			}()
			return nil
		}
		return err
	}
	return nil
}
