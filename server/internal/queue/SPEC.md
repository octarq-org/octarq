# SPEC: Task Queue with Memory & Asynq Workers (`server/internal/queue`)

## 1. Overview & Architecture
`server/internal/queue` provides asynchronous background task processing for Octarq Core.
- Provides a unified `Queue` interface supporting both local in-memory buffered workers and distributed Redis-backed `asynq` workers.
- Uses `InMemoryQueue` only when `OCTARQ_REDIS_URL` is unset.
- Integrates OpenTelemetry distributed tracing and metrics (`queue.process`, `QueueTasksInFlight`, execution duration).
- Fail-closed enqueue: a failed Redis enqueue returns an error to the caller, and a full in-memory buffer returns an error. Neither case runs the task inline on the enqueueing instance.

## 2. Exported Contracts & Types
- `Queue`: Interface declaring:
  - `Register(taskType string, h Handler)`: Register a worker callback for a task type.
  - `Start(ctx context.Context) error`: Start task processing workers until context cancellation.
  - `Enqueue(ctx context.Context, taskType string, payload []byte) error`: Enqueue a payload with trace carrier.
- `Handler`: `func(ctx context.Context, payload []byte) error`.
- `New(redisURL string) Queue`: Factory returning `AsynqQueue` (if valid Redis URL) or `InMemoryQueue`.
- `InMemoryQueue`: Channel-backed local worker queue with 4 worker goroutines.
- `AsynqQueue`: Redis-backed distributed task queue.

## 3. Reliability & Invariant Guarantees
- **Trace Context Propagation**: OpenTelemetry trace context is injected into a `taskEnvelope` carrier during `Enqueue` and extracted during worker execution.
- **Graceful Shutdown**: Shuts down consumers cleanly when context is canceled, closing the client connection without leaking resources.
- **No Inline Execution on Enqueue Failure**: when Redis is configured, `AsynqQueue.Enqueue` wraps and returns the Redis error. The enqueueing instance never runs the handler inline, because a broken Redis deployment must stay visible to the caller.
- **Backpressure Protection**: `InMemoryQueue.Enqueue` returns a `queue: full` error once the 1000-task buffer is exhausted. It never blocks and never runs the handler inline.

## 4. File Layout
- `queue.go`: `Queue` interface, `InMemoryQueue`, `AsynqQueue`, telemetry instrumentation, and lifecycle.
- `queue_test.go`: Unit tests for memory and asynq queue implementations.
- `queue_characterization_test.go`: Characterization tests locking dispatch, backpressure, and unregistered tasks.
