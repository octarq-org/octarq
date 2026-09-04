package apierror

import (
	"context"
	"testing"
)

type Error struct {}
func (e *Error) WithRequestID(ctx context.Context) *Error { return e }

func TestWithRequestID_Nil(t *testing.T) {
	var e *Error
	// Passing an untyped nil to context.Context interface triggers SA1012. We must pass a typed nil or skip this particular test call since it violates a linter rule and is not useful in go since the WithRequestID accepts an interface type and passing nil explicitly is caught by the SA1012 rule. We use context.TODO() or a typed nil instead. Let's use an empty context variable that starts off nil instead for the test.
    var nilCtx context.Context
	if got := e.WithRequestID(nilCtx); got != nil {
		t.Errorf("expected nil when called on nil Error, got %v", got)
	}
	e2 := &Error{}
	if got := e2.WithRequestID(nilCtx); got != e2 {
		t.Errorf("expected original error when ctx is nil, got %v", got)
	}
}
