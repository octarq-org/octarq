package hello_test

import (
	"context"
	"testing"

	"github.com/octarq-org/octarq/plugin"
)

type helloInput struct {
	Name string `json:"name"`
}

type helloOutput struct {
	Message string `json:"message"`
}

func TestHelloEndpointExample(t *testing.T) {
	spec := plugin.EndpointSpec[helloInput, helloOutput]{
		Name:        "hello_greet",
		Summary:     "Greet someone",
		Description: "Returns a friendly greeting message",
		Method:      "POST",
		Path:        "/api/hello/greet",
		RequireAuth: false,
		ExposeMCP:   true,
		Handler: func(ctx context.Context, input helloInput) (*helloOutput, error) {
			who := input.Name
			if who == "" {
				who = "world"
			}
			return &helloOutput{Message: "hello, " + who + "!"}, nil
		},
	}

	if err := spec.Validate(); err != nil {
		t.Fatalf("expected valid EndpointSpec, got error: %v", err)
	}

	res, err := spec.Execute(context.Background(), helloInput{Name: "developer"})
	if err != nil {
		t.Fatalf("failed to execute endpoint: %v", err)
	}
	out, ok := res.(*helloOutput)
	if !ok || out.Message != "hello, developer!" {
		t.Fatalf("unexpected result: %+v", res)
	}
}
