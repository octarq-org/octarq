package notify

import (
	"context"
	"testing"
)

func TestSendMalformedJSONErrors(t *testing.T) {
	if err := Send(context.Background(), "webhook", "not-json", "text"); err == nil {
		t.Fatal("expected error for malformed JSON config")
	}
}
