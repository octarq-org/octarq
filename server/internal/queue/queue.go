package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/hibiken/asynq"
	"github.com/octarq-org/octarq/pkg/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Handler represents a task worker function.
type Handler func(ctx context.Context, payload []byte) error

// Queue defines a robust, optional task queue.
type Queue interface {
	Register(taskType string, h Handler)
	Start(ctx context.Context) error
	Enqueue(ctx context.Context, taskType string, payload []byte) error
}

type taskEnvelope struct {
	Carrier map[string]string `json:"_trace,omitempty"`
	Payload []byte            `json:"payload"`
}

func wrapPayload(ctx context.Context, payload []byte) []byte {
	carrier := make(map[string]string)
	telemetry.InjectMap(ctx, carrier)
	if len(carrier) == 0 {
		carrier = nil
	}
	env := taskEnvelope{
		Carrier: carrier,
		Payload: payload,
	}
	b, err := json.Marshal(env)
	if err != nil {
		return payload
	}
	return b
}

func runHandlerWithTelemetry(taskType string, h Handler, raw []byte) error {
	var env taskEnvelope
	var ctx context.Context
	var payload []byte

	if err := json.Unmarshal(raw, &env); err == nil && (len(env.Carrier) > 0 || len(env.Payload) > 0) {
		ctx = telemetry.ExtractMap(context.Background(), env.Carrier)
		payload = env.Payload
	} else {
		ctx = context.Background()
		payload = raw
	}

	ctx, span := telemetry.StartSpan(ctx, "github.com/octarq-org/octarq/queue", "queue.process "+taskType,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("task.type", taskType),
		),
	)
	defer span.End()

	tel := telemetry.Global()
	if tel.Metrics != nil && tel.Metrics.QueueTasksInFlight != nil {
		tel.Metrics.QueueTasksInFlight.Add(ctx, 1)
		defer tel.Metrics.QueueTasksInFlight.Add(ctx, -1)
	}

	start := time.Now()
	err := h(ctx, payload)
	duration := time.Since(start)

	if err != nil {
		telemetry.RecordError(span, err)
		if tel.Metrics != nil {
			tel.Metrics.RecordQueueTask(ctx, taskType, "error", duration)
		}
	} else {
		telemetry.SetOK(span)
		if tel.Metrics != nil {
			tel.Metrics.RecordQueueTask(ctx, taskType, "success", duration)
		}
	}
	return err
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

type InMemoryQueue struct {
	mu       sync.RWMutex
	handlers map[string]Handler
	ch       chan task
	wg       sync.WaitGroup
}

func (q *InMemoryQueue) handler(taskType string) (Handler, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	h, ok := q.handlers[taskType]
	return h, ok
}

func newInMemoryQueue() *InMemoryQueue {
	return &InMemoryQueue{
		handlers: make(map[string]Handler),
		ch:       make(chan task, 1000),
	}
}

func (q *InMemoryQueue) Register(taskType string, h Handler) {
	q.mu.Lock()
	defer q.mu.Unlock()
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
					h, exists := q.handler(t.Type)
					if !exists {
						log.Printf("queue: no handler registered for task type %q", t.Type)
						continue
					}
					if err := runHandlerWithTelemetry(t.Type, h, t.Payload); err != nil {
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
	wrapped := wrapPayload(ctx, payload)
	select {
	case q.ch <- task{Type: taskType, Payload: wrapped}:
		return nil
	default:
		// Queue full, fallback to instant goroutine execution
		h, exists := q.handler(taskType)
		if !exists {
			return fmt.Errorf("queue: full and no handler registered for %q", taskType)
		}
		go func() {
			if err := runHandlerWithTelemetry(taskType, h, wrapped); err != nil {
				log.Printf("queue: fallback instant execution of %q failed: %v", taskType, err)
			}
		}()
		return nil
	}
}

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
	wrapped := wrapPayload(ctx, payload)
	t := asynq.NewTask(taskType, wrapped)
	_, err := q.client.EnqueueContext(ctx, t)
	if err != nil {
		log.Printf("queue: asynq Enqueue failed: %v. Falling back to instant execution.", err)
		// Connection failed - run instantly in background as fallback
		q.mu.Lock()
		h, exists := q.handlers[taskType]
		q.mu.Unlock()
		if exists {
			go func() {
				if err := runHandlerWithTelemetry(taskType, h, wrapped); err != nil {
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
		tType := taskType
		mux.HandleFunc(taskType, func(ctx context.Context, t *asynq.Task) error {
			return runHandlerWithTelemetry(tType, handler, t.Payload())
		})
	}

	if err := srv.Start(mux); err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		srv.Shutdown()
		_ = q.client.Close()
	}()

	return nil
}
