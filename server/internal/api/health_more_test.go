package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
)

func TestHealthAndSubsystems(t *testing.T) {
	h, srv, db := newTestHandlerRaw(t)

	rec := do(srv, "GET", "/api/status", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/status: got %d (%s)", rec.Code, rec.Body.String())
	}

	db.Create(&testSMTPSender{Host: "smtp.example.com", Port: 587, OrgID: 1})
	rec = do(srv, "GET", "/api/status", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/status with smtp: got %d", rec.Code)
	}

	h.statusLimiter = mustRateLimiter(t, "", "status", 2, time.Minute)
	h.statusLimiter.recordFailure("192.0.2.1")
	h.statusLimiter.recordFailure("192.0.2.1")
	req := httptest.NewRequest("GET", "/api/status", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	rec2 := httptest.NewRecorder()
	srv.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 for rate-limited status, got %d", rec2.Code)
	}

	hNilDB := &Handler{db: nil}
	out, err := hNilDB.subsystemStatus(context.Background(), &StatusStatusInput{Ctx: nil})
	if err != nil || out.Body.Overall != "down" {
		t.Errorf("subsystemStatus with nil db = %+v, err=%v", out, err)
	}

	_, srv2, db2 := newTestHandlerRaw(t)
	sqlDB, _ := db2.DB()
	sqlDB.Close()
	reqClosed := httptest.NewRequest("GET", "/api/health", nil)
	recClosed := httptest.NewRecorder()
	srv2.ServeHTTP(recClosed, reqClosed)
	if recClosed.Code != http.StatusOK {
		t.Errorf("liveness probe failed while the database was down: got %d", recClosed.Code)
	}

	ctx := context.Background()
	if out, err := h.health(ctx, &HealthInput{Ctx: nil}); err != nil {
		t.Errorf("health with nil Ctx = %v, want no error", err)
	} else if out == nil {
		t.Error("health with nil Ctx returned no output")
	}
}

func TestStatusReportsMessagingReadiness(t *testing.T) {
	h, _, db := newTestHandlerRaw(t)

	readMail := func(h *Handler) string {
		t.Helper()
		out, err := h.subsystemStatus(context.Background(), &StatusStatusInput{})
		if err != nil {
			t.Fatalf("subsystemStatus: %v", err)
		}
		for _, s := range out.Body.Subsystems {
			if s.Name == "mail" {
				return s.Status
			}
		}
		t.Fatalf("subsystems = %+v, want a mail entry", out.Body.Subsystems)
		return ""
	}

	if got := readMail(h); got != "degraded" {
		t.Errorf("mail = %q with a message plugin and no sender, want degraded", got)
	}

	sender := testSMTPSender{Host: "smtp.example.com", Port: 587, OrgID: 1}
	db.Create(&sender)
	db.Create(&models.Setting{Key: "mail_system_sender_id", Value: fmt.Sprint(sender.ID)})
	if got := readMail(h); got != "ok" {
		t.Errorf("mail = %q with a ready sender, want ok", got)
	}

	if got := readMail(&Handler{db: db}); got != "na" {
		t.Errorf("mail = %q without the messaging service, want na", got)
	}
}
