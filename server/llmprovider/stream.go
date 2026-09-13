package llmprovider

import (
	"context"
	"errors"
	"fmt"
)

// ErrStreamOrder is returned when a stream sends events in an illegal sequence,
// such as a thinking delta arriving after text or tool-call content has already begun.
var ErrStreamOrder = errors.New("llmprovider: stream order violation")

// ToolCallChunk is a partial fragment of a tool call streamed from the model.
type ToolCallChunk struct {
	Index    int    // 并行多工具序号 (0-based index for parallel/sequential tool calls)
	ID       string // 首个分片携带 (tool use identifier)
	Name     string // tool name
	ArgsJSON string // 参数 JSON 分片（原样转发，由调用方累积）
}

// StreamHandler receives incremental updates during a streaming completion.
type StreamHandler interface {
	OnText(delta string)
	OnThinking(delta string)
	OnToolCall(chunk ToolCallChunk)
}

// StreamProvider is an optional interface implemented by providers that support streaming.
type StreamProvider interface {
	StreamComplete(ctx context.Context, req Request, h StreamHandler) (Response, error)
}

// StreamOrderGuard enforces the legal event sequence:
// Thinking* -> (Text* | ToolCallChunk*) -> End.
// Once non-empty text or tool-call content has been emitted, receiving a thinking
// delta violates the protocol and fails closed.
type StreamOrderGuard struct {
	sawContent bool
}

type streamOrderGuard = StreamOrderGuard

// OnThinking checks whether a thinking delta is allowed at the current point in
// the stream. If content has already been seen, it returns an error wrapping ErrStreamOrder.
func (g *streamOrderGuard) OnThinking(delta string) error {
	if delta == "" {
		return nil
	}
	if g.sawContent {
		return fmt.Errorf("%w: thinking delta received after text/tool content", ErrStreamOrder)
	}
	return nil
}

// OnText marks that text content has arrived.
func (g *streamOrderGuard) OnText(delta string) {
	if delta != "" {
		g.sawContent = true
	}
}

// OnToolCall marks that tool-call content has arrived.
func (g *streamOrderGuard) OnToolCall(_ ToolCallChunk) {
	g.sawContent = true
}
