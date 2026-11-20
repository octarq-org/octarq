package llmgateway

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type listCall struct {
	path  string
	key   string
	auth  string
	query string
}

type anthropicList struct {
	mu           sync.Mutex
	pages        []string
	requests     []listCall
	code         int
	repeatLastID bool
}

func (a *anthropicList) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	a.requests = append(a.requests, listCall{
		path:  r.URL.Path,
		key:   r.Header.Get("x-api-key"),
		auth:  r.Header.Get("Authorization"),
		query: r.URL.RawQuery,
	})
	n := len(a.requests) - 1
	a.mu.Unlock()
	code := a.code
	if code == 0 {
		code = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if a.repeatLastID {
		_, _ = io.WriteString(w, `{"data":[{"id":"claude-a"}],"has_more":true,"last_id":"claude-a"}`)
		return
	}
	if n >= len(a.pages) {
		_, _ = io.WriteString(w, a.pages[len(a.pages)-1])
		return
	}
	_, _ = io.WriteString(w, a.pages[n])
}

func TestAnthropicModelsReadsEveryPage(t *testing.T) {
	list := &anthropicList{pages: []string{
		`{"data":[{"id":"claude-a","type":"model"},{"id":"claude-b"}],"has_more":true,"last_id":"claude-b"}`,
		`{"data":[{"id":"claude-c"}],"has_more":false,"last_id":"claude-c"}`,
	}}
	srv := httptest.NewServer(list)
	t.Cleanup(srv.Close)

	models, err := anthropicModels(context.Background(), agentProvider{BaseURL: srv.URL, APIKey: "sk-ant-test"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got := make([]string, 0, len(models))
	for _, m := range models {
		got = append(got, m.ID)
		if m.Object != "model" {
			t.Fatalf("%s object = %q, want model", m.ID, m.Object)
		}
	}
	if strings.Join(got, ",") != "claude-a,claude-b,claude-c" {
		t.Fatalf("models = %v", got)
	}
	list.mu.Lock()
	defer list.mu.Unlock()
	if len(list.requests) != 2 {
		t.Fatalf("pages read = %d, want 2", len(list.requests))
	}
	if list.requests[0].path != "/v1/models" {
		t.Fatalf("path = %q, want /v1/models", list.requests[0].path)
	}
	if list.requests[0].key != "sk-ant-test" {
		t.Fatalf("key = %q; Anthropic reads x-api-key, not a bearer token", list.requests[0].key)
	}
	if list.requests[0].auth != "" {
		t.Fatalf("Authorization = %q; Anthropic does not take one here", list.requests[0].auth)
	}
	if !strings.Contains(list.requests[1].query, "after_id=claude-b") {
		t.Fatalf("second page query = %q, want the cursor", list.requests[1].query)
	}
}

func TestAnthropicModelsReportsARefusal(t *testing.T) {
	list := &anthropicList{code: http.StatusUnauthorized, pages: []string{`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`}}
	srv := httptest.NewServer(list)
	t.Cleanup(srv.Close)

	_, err := anthropicModels(context.Background(), agentProvider{BaseURL: srv.URL, APIKey: "sk-ant-bad"})
	if err == nil {
		t.Fatal("a refused list was accepted")
	}
	if !strings.Contains(err.Error(), "refused the model list") || !strings.Contains(err.Error(), "invalid x-api-key") {
		t.Fatalf("err = %v; the provider's reason must survive", err)
	}
}

func TestAnthropicModelsStopsOnARepeatingPage(t *testing.T) {
	list := &anthropicList{repeatLastID: true}
	srv := httptest.NewServer(list)
	t.Cleanup(srv.Close)

	models, err := anthropicModels(context.Background(), agentProvider{BaseURL: srv.URL, APIKey: "k"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(models) != 1 || models[0].ID != "claude-a" {
		t.Fatalf("models = %+v", models)
	}
	list.mu.Lock()
	defer list.mu.Unlock()
	if len(list.requests) != 2 {
		t.Fatalf("pages read = %d, want 2", len(list.requests))
	}
}

func TestAnthropicModelsRefusesAnEndlessList(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"claude-`+strconv.Itoa(n)+`"}],"has_more":true,"last_id":"claude-`+strconv.Itoa(n)+`"}`)
	}))
	t.Cleanup(srv.Close)

	_, err := anthropicModels(context.Background(), agentProvider{BaseURL: srv.URL, APIKey: "k"})
	if err == nil {
		t.Fatal("a list that never ends was accepted")
	}
	if !strings.Contains(err.Error(), "did not end") {
		t.Fatalf("err = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls > 10 {
		t.Fatalf("pages read = %d, want the reader to stop within ten", calls)
	}
	if calls < 2 {
		t.Fatalf("pages read = %d, want the reader to follow the cursor", calls)
	}
}

func TestDiscoveryResolutionReadsTheAnthropicAPI(t *testing.T) {
	p := &Plugin{}
	ctx := context.Background()
	ap, err := p.discoveryProviderFromRow(ctx, &Provider{Backend: "claude"}, "sk-ant")
	if err != nil {
		t.Fatalf("claude row: %v", err)
	}
	if !ap.Anthropic || ap.BaseURL != anthropicBaseURL {
		t.Fatalf("claude row resolved to %+v", ap)
	}
	ap, err = p.discoveryProviderFromRow(ctx, &Provider{Backend: "claude", BaseURL: "https://proxy.example/v1"}, "sk-ant")
	if err != nil {
		t.Fatalf("claude row with a base URL: %v", err)
	}
	if ap.Anthropic || ap.BaseURL != "https://proxy.example/v1" {
		t.Fatalf("proxied claude row resolved to %+v", ap)
	}
	ap, err = p.discoveryProviderFromRow(ctx, &Provider{Backend: "claude", BaseURL: "https://api.anthropic.com/v1"}, "sk-ant")
	if err != nil {
		t.Fatalf("pasted anthropic URL: %v", err)
	}
	if !ap.Anthropic || ap.BaseURL != anthropicBaseURL {
		t.Fatalf("pasted anthropic URL resolved to %+v", ap)
	}
	if _, err := p.discoveryProviderFromRow(ctx, &Provider{Backend: "gemini"}, "k"); !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("gemini err = %v, want ErrInvalidProvider", err)
	}
}
