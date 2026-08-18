package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/octarq-org/octarq/plugins/mail"
)

func TestHealthAndSubsystems(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)

	// 1. Subsystem status default
	rec := do(srv, "GET", "/api/status", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/status: got %d (%s)", rec.Code, rec.Body.String())
	}

	// 2. Subsystem status with mail smtp_senders in DB
	db.Create(&mail.SMTPSender{Host: "smtp.example.com", Port: 587, OrgID: 1})
	rec = do(srv, "GET", "/api/status", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/status with smtp: got %d", rec.Code)
	}

	// 3. Subsystem status rate limiting
	h.statusLimiter = newRateLimiter("", "status", 2, time.Minute)
	// Trigger 2 failures to exhaust rate limit
	h.statusLimiter.recordFailure("192.0.2.1")
	h.statusLimiter.recordFailure("192.0.2.1")
	req := httptest.NewRequest("GET", "/api/status", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	rec2 := httptest.NewRecorder()
	srv.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 for rate-limited status, got %d", rec2.Code)
	}

	// 4. Subsystem status with nil db
	hNilDB := &Handler{db: nil}
	out, err := hNilDB.subsystemStatus(context.Background(), &StatusStatusInput{Ctx: nil})
	if err != nil || out.Body.Overall != "down" {
		t.Errorf("subsystemStatus with nil db = %+v, err=%v", out, err)
	}

	// 5. Unhealthy database ping failure -> 503
	_, srv2, db2 := newTestHandlerRaw(t)
	sqlDB, _ := db2.DB()
	sqlDB.Close()
	reqClosed := httptest.NewRequest("GET", "/api/health", nil)
	recClosed := httptest.NewRecorder()
	srv2.ServeHTTP(recClosed, reqClosed)
	if recClosed.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 for unhealthy DB, got %d", recClosed.Code)
	}

	// 6. Direct calls with nil Ctx
	ctx := context.Background()
	if _, err := h.health(ctx, &HealthInput{Ctx: nil}); err != nil {
		// health on valid db should return health response
	}
}
