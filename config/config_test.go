package config

import "testing"

func TestLoadRequiresSecretKey(t *testing.T) {
	t.Setenv("LED_SECRET_KEY", "")
	t.Setenv("LED_ADMIN_PASSWORD", "pw")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when LED_SECRET_KEY is unset")
	}
}

func TestLoadRequiresAdminPassword(t *testing.T) {
	t.Setenv("LED_SECRET_KEY", "s")
	t.Setenv("LED_ADMIN_PASSWORD", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when LED_ADMIN_PASSWORD is unset")
	}
}

func TestLoadRejectsBadDriver(t *testing.T) {
	t.Setenv("LED_SECRET_KEY", "s")
	t.Setenv("LED_ADMIN_PASSWORD", "pw")
	t.Setenv("LED_DB_DRIVER", "mysql")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for unsupported driver")
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	t.Setenv("LED_SECRET_KEY", "s")
	t.Setenv("LED_ADMIN_PASSWORD", "pw")
	t.Setenv("LED_DB_DRIVER", "sqlite")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Listen != ":8080" {
		t.Errorf("Listen default = %q want :8080", cfg.Listen)
	}
	if cfg.AdminUser != "admin" {
		t.Errorf("AdminUser default = %q want admin", cfg.AdminUser)
	}
	if !cfg.CatchAll {
		t.Error("CatchAll default should be true")
	}
	if cfg.DBDSN != "led.db" {
		t.Errorf("DBDSN default = %q want led.db", cfg.DBDSN)
	}
}

func TestLoadTrimsBaseURL(t *testing.T) {
	t.Setenv("LED_SECRET_KEY", "s")
	t.Setenv("LED_ADMIN_PASSWORD", "pw")
	t.Setenv("LED_BASE_URL", "https://go.example.com/")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BaseURL != "https://go.example.com" {
		t.Errorf("BaseURL = %q, trailing slash not trimmed", cfg.BaseURL)
	}
}
