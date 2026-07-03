package queue

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/hibiken/asynq"
)

// Handler represents a task worker function.
type Handler func(ctx context.Context, payload []byte) error

// Queue defines a robust, optional task queue.
type Queue interface {
	Register(taskType string, h Handler)
	Start(ctx context.Context) error
	Enqueue(ctx context.Context, taskType string, payload []byte) error
}

// New returns a Queue implementation. If redisURL is empty or fails to parse,
// it returns a local InMemoryQueue. Otherwise, it returns an Asynq-backed Queue.
func New(redisURL string) Queue {
	if redisURL == "" {
		log.Println("queue: Redis URL is empty. Using in-memory task queue.")
		return newInMemoryQueue()
	}

	redisOpt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		log.Printf("queue: failed to parse redis URL %q: %v. Falling back to in-memory queue.", redisURL, err)
		return newInMemoryQueue()
	}

	log.Printf("queue: initialized Asynq task queue with Redis")
	return &AsynqQueue{
		redisOpt: redisOpt,
		client:   asynq.NewClient(redisOpt),
		handlers: make(map[string]Handler),
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

func newInMemoryQueue() *InMemoryQueue {
	return &InMemoryQueue{
		handlers: make(map[string]Handler),
		ch:       make(chan task, 1000),
	}
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

// --- AsynqQueue implementation ---

type AsynqQueue struct {
	redisOpt asynq.RedisConnOpt
	client   *asynq.Client
	server   *asynq.Server
	handlers map[string]Handler
	mu       sync.Mutex
}

func (q *AsynqQueue) Register(taskType string, h Handler) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.handlers[taskType] = h
}

func (q *AsynqQueue) Enqueue(ctx context.Context, taskType string, payload []byte) error {
	t := asynq.NewTask(taskType, payload)
	_, err := q.client.EnqueueContext(ctx, t)
	if err != nil {
		log.Printf("queue: asynq Enqueue failed: %v. Falling back to instant execution.", err)
		// Connection failed - run instantly in background as fallback
		q.mu.Lock()
		h, exists := q.handlers[taskType]
		q.mu.Unlock()
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

func (q *AsynqQueue) Start(ctx context.Context) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	srv := asynq.NewServer(q.redisOpt, asynq.Config{
		Concurrency: 4,
	})
	q.server = srv

	mux := asynq.NewServeMux()
	for taskType, h := range q.handlers {
		handler := h // capture loop variable
		mux.HandleFunc(taskType, func(ctx context.Context, t *asynq.Task) error {
			return handler(ctx, t.Payload())
		})
	}

	if err := srv.Start(mux); err != nil {
		return err
	}

	// Shut down workers gracefully on context done
	go func() {
		<-ctx.Done()
		srv.Shutdown()
		_ = q.client.Close()
	}()

	return nil
}
