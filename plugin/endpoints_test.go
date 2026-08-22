package plugin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/octarq-org/octarq/plugin"
)

type echoIn struct {
	Message string `json:"message"`
}

type echoOut struct {
	Reply string `json:"reply"`
}

func TestEndpointSpec_Execute(t *testing.T) {
	spec := plugin.EndpointSpec[echoIn, echoOut]{
		Name:        "echo",
		Summary:     "Echoes a message",
		Description: "Returns the input message in reply field",
		Method:      "POST",
		Path:        "/api/echo",
		RequireAuth: true,
		RequireRole: []string{"admin"},
		ExposeMCP:   true,
		Handler: func(ctx context.Context, input echoIn) (*echoOut, error) {
			if input.Message == "fail" {
				return nil, errors.New("forced failure")
			}
			return &echoOut{Reply: "echo: " + input.Message}, nil
		},
	}

	if spec.EndpointName() != "echo" {
		t.Errorf("expected Name 'echo', got %q", spec.EndpointName())
	}
	if spec.EndpointMethod() != "POST" {
		t.Errorf("expected Method 'POST', got %q", spec.EndpointMethod())
	}
	if spec.EndpointPath() != "/api/echo" {
		t.Errorf("expected Path '/api/echo', got %q", spec.EndpointPath())
	}
	if !spec.EndpointRequireAuth() {
		t.Errorf("expected RequireAuth true")
	}
	if len(spec.EndpointRequireRole()) != 1 || spec.EndpointRequireRole()[0] != "admin" {
		t.Errorf("expected RequireRole ['admin'], got %v", spec.EndpointRequireRole())
	}
	if !spec.EndpointExposeMCP() {
		t.Errorf("expected ExposeMCP true")
	}

	// Success case
	res, err := spec.Execute(context.Background(), echoIn{Message: "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, ok := res.(*echoOut)
	if !ok || out.Reply != "echo: hello" {
		t.Fatalf("expected 'echo: hello', got %+v", res)
	}

	// Pointer input conversion
	resPtr, err := spec.Execute(context.Background(), &echoIn{Message: "ptr"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	outPtr, ok := resPtr.(*echoOut)
	if !ok || outPtr.Reply != "echo: ptr" {
		t.Fatalf("expected 'echo: ptr', got %+v", resPtr)
	}

	// Error case
	_, err = spec.Execute(context.Background(), echoIn{Message: "fail"})
	if err == nil || err.Error() != "forced failure" {
		t.Fatalf("expected 'forced failure' error, got %v", err)
	}

	// Incompatible type error
	_, err = spec.Execute(context.Background(), 12345)
	if err == nil {
		t.Fatalf("expected error for incompatible input type")
	}
}

func TestRegisterEndpoint_NilContext(t *testing.T) {
	spec := plugin.EndpointSpec[echoIn, echoOut]{
		Name: "echo",
	}
	// Calling with nil Context should return nil without panic
	if err := plugin.RegisterEndpoint[echoIn, echoOut](nil, spec); err != nil {
		t.Errorf("expected nil error on nil context, got %v", err)
	}

	// Calling with uninitialized RegisterEndpoint func
	ctx := &plugin.Context{}
	if err := plugin.RegisterEndpoint(ctx, spec); err != nil {
		t.Errorf("expected nil error on uninitialized RegisterEndpoint, got %v", err)
	}

	// Calling with registered handler
	var registered any
	ctx.RegisterEndpoint = func(s any) error {
		registered = s
		return nil
	}
	if err := plugin.RegisterEndpoint(ctx, spec); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if registered == nil {
		t.Errorf("expected spec to be passed to RegisterEndpoint")
	}
}
