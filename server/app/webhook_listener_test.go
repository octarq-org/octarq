package app

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestWebhookDispatcherRouting(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		wantCalled bool
		wantStatus int
	}{
		{
			name:       "allowed webhook endpoint",
			method:     http.MethodPost,
			path:       "/api/connectors/webhook",
			wantCalled: true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "allowed health endpoint",
			method:     http.MethodGet,
			path:       "/api/health",
			wantCalled: true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "disallowed bots endpoint",
			method:     http.MethodGet,
			path:       "/api/bots",
			wantCalled: false,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "disallowed login endpoint",
			method:     http.MethodPost,
			path:       "/api/auth/login",
			wantCalled: false,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "disallowed admin endpoint",
			method:     http.MethodGet,
			path:       "/admin/",
			wantCalled: false,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "disallowed schemas endpoint",
			method:     http.MethodGet,
			path:       "/schemas/endpoint.json",
			wantCalled: false,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "disallowed method on webhook endpoint",
			method:     http.MethodGet,
			path:       "/api/connectors/webhook",
			wantCalled: false,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "disallowed method on health endpoint",
			method:     http.MethodPost,
			path:       "/api/health",
			wantCalled: false,
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			stub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})

			dispatcher := newWebhookDispatcher(stub)
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()

			dispatcher.ServeHTTP(rec, req)

			if called != tt.wantCalled {
				t.Errorf("stub called = %v, want %v", called, tt.wantCalled)
			}
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestWebhookListenConflictValidation(t *testing.T) {
	err := validateWebhookListen(":8080", ":8080")
	if err == nil {
		t.Fatal("expected conflict error when WebhookListen == Listen, got nil")
	}
}

func TestWebhookListenerFailClosedOccupiedPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen ephemeral port: %v", err)
	}
	defer ln.Close()

	occupiedAddr := ln.Addr().String()

	tempDir := t.TempDir()
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", filepath.Join(tempDir, "occupied.db"))
	t.Setenv("OCTARQ_SECRET_KEY", "app-boot-secret-key-32-bytes-long!!!")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "app-boot-admin-pass")
	t.Setenv("OCTARQ_LISTEN", "127.0.0.1:0")
	t.Setenv("OCTARQ_WEBHOOK_LISTEN", occupiedAddr)

	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := a.Run(ctx); err == nil {
		t.Fatal("expected Run to fail closed when webhook port is occupied, got nil")
	}
}
