package app

import (
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

func TestAppMethods(t *testing.T) {

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "app_methods.db")
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", dbPath)
	t.Setenv("OCTARQ_SECRET_KEY", "test-secret-key-16-bytes")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "test-admin-pass")

	a, err := New()
	if err != nil {
		t.Fatalf("app.New() failed: %v", err)
	}

	if a.DB() == nil {
		t.Error("DB() is nil")
	}
	p1 := testDummyPlugin{name: "p1"}
	a.Use(p1)
	if len(a.Plugins()) != 1 {
		t.Errorf("Plugins() length = %d, want 1", len(a.Plugins()))
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/auth/login", nil)
	_, _ = a.loginByEmail(w, r, "admin@example.com")

	if err := a.sendMail(1, "to@example.com", "sub", "<p>hi</p>", "hi"); err == nil {
		t.Error("expected error from sendMail when no mail plugin mounted")
	}

	a.services = plugin.NewRegistry()
	a.services.Provide(plugin.ServiceMessagingSend, plugin.MessagingSender(func(orgID uint, to, subject, html, text string) error {
		return nil
	}))
	if err := a.sendMail(1, "to@example.com", "sub", "<p>hi</p>", "hi"); err != nil {
		t.Errorf("sendMail error: %v", err)
	}
}
