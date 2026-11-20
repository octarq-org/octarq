package llmprovider

import (
	"context"
	"errors"
	"fmt"
)

var ErrStreamOrder = errors.New("llmprovider: stream order violation")

type ToolCallChunk struct {
	Index    int
	ID       string
	Name     string
	ArgsJSON string
}

type StreamHandler interface {
	OnText(delta string)
	OnThinking(delta string)
	OnToolCall(chunk ToolCallChunk)
}

type StreamProvider interface {
	StreamComplete(ctx context.Context, req Request, h StreamHandler) (Response, error)
}

type StreamOrderGuard struct {
	sawContent bool
}

type streamOrderGuard = StreamOrderGuard

func (g *streamOrderGuard) OnThinking(delta string) error {
	if delta == "" {
		return nil
	}
	if g.sawContent {
		return fmt.Errorf("%w: thinking delta received after text/tool content", ErrStreamOrder)
	}
	return nil
}

func (g *streamOrderGuard) OnText(delta string) {
	if delta != "" {
		g.sawContent = true
	}
}

func (g *streamOrderGuard) OnToolCall(_ ToolCallChunk) {
	g.sawContent = true
}
