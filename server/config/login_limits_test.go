package config

import (
	"path/filepath"
	"testing"
	"time"
)

// loginEnv sets the minimum a Load needs, so a test can vary one login budget.
func loginEnv(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("OCTARQ_SECRET_KEY", "1234567890123456")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "octarq-test-pw")
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", filepath.Join(dir, "octarq.db"))
}

func TestLoginBudgetsDefaultToTheProductionValues(t *testing.T) {
	loginEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LoginMaxFails != 5 {
		t.Errorf("LoginMaxFails = %d, want 5", cfg.LoginMaxFails)
	}
	if cfg.LoginAccountMaxFails != 10 {
		t.Errorf("LoginAccountMaxFails = %d, want 10", cfg.LoginAccountMaxFails)
	}
	if cfg.LoginWindow != 15*time.Minute {
		t.Errorf("LoginWindow = %v, want 15m", cfg.LoginWindow)
	}
}

func TestLoginBudgetsFromEnvironment(t *testing.T) {
	loginEnv(t)
	t.Setenv("OCTARQ_LOGIN_MAX_FAILS", "40")
	t.Setenv("OCTARQ_LOGIN_ACCOUNT_MAX_FAILS", "80")
	t.Setenv("OCTARQ_LOGIN_WINDOW", "2m")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LoginMaxFails != 40 || cfg.LoginAccountMaxFails != 80 {
		t.Fatalf("budgets = %d/%d, want 40/80", cfg.LoginMaxFails, cfg.LoginAccountMaxFails)
	}
	if cfg.LoginWindow != 2*time.Minute {
		t.Fatalf("LoginWindow = %v, want 2m", cfg.LoginWindow)
	}
}

// A value nobody can read stops startup. Falling back to the default would
// leave an operator with a lockout, and no sign of why the number did nothing.
func TestLoginBudgetsRefuseAValueTheyCannotRead(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"OCTARQ_LOGIN_MAX_FAILS", "0"},
		{"OCTARQ_LOGIN_MAX_FAILS", "ten"},
		{"OCTARQ_LOGIN_ACCOUNT_MAX_FAILS", "-3"},
		{"OCTARQ_LOGIN_WINDOW", "0s"},
		{"OCTARQ_LOGIN_WINDOW", "15"},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			loginEnv(t)
			t.Setenv(tc.key, tc.value)

			if _, err := Load(); err == nil {
				t.Fatalf("Load accepted %s=%q", tc.key, tc.value)
			}
		})
	}
}
