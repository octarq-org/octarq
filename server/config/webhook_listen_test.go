package config

import (
	"path/filepath"
	"testing"
)

func TestWebhookListenParsing(t *testing.T) {
	t.Setenv("OCTARQ_DB_DRIVER", "sqlite")
	t.Setenv("OCTARQ_DB_DSN", filepath.Join(t.TempDir(), "test.db"))
	t.Setenv("OCTARQ_SECRET_KEY", "webhook-listen-test-secret-key-12345")
	t.Setenv("OCTARQ_ADMIN_PASSWORD", "webhook-listen-test-admin-pw")

	t.Run("configured parses into WebhookListen", func(t *testing.T) {
		t.Setenv("OCTARQ_WEBHOOK_LISTEN", "127.0.0.1:9001")
		cfg, err := LoadWithOptions(LoadOptions{DotEnvPath: ""})
		if err != nil {
			t.Fatalf("LoadWithOptions failed: %v", err)
		}
		if cfg.WebhookListen != "127.0.0.1:9001" {
			t.Errorf("WebhookListen = %q, want 127.0.0.1:9001", cfg.WebhookListen)
		}
	})

	t.Run("unset resolves to empty", func(t *testing.T) {
		t.Setenv("OCTARQ_WEBHOOK_LISTEN", "")
		cfg, err := LoadWithOptions(LoadOptions{DotEnvPath: ""})
		if err != nil {
			t.Fatalf("LoadWithOptions failed: %v", err)
		}
		if cfg.WebhookListen != "" {
			t.Errorf("WebhookListen = %q, want empty", cfg.WebhookListen)
		}
	})
}

func TestValidateWebhookListen(t *testing.T) {
	tests := []struct {
		name          string
		listen        string
		webhookListen string
		wantErr       bool
	}{
		{
			name:          "empty webhook listen is valid",
			listen:        ":8080",
			webhookListen: "",
			wantErr:       false,
		},
		{
			name:          "distinct addresses are valid",
			listen:        ":8080",
			webhookListen: ":9001",
			wantErr:       false,
		},
		{
			name:          "exact conflict returns error",
			listen:        ":8080",
			webhookListen: ":8080",
			wantErr:       true,
		},
		{
			name:          "conflict with 127.0.0.1 returns error",
			listen:        "127.0.0.1:9001",
			webhookListen: "127.0.0.1:9001",
			wantErr:       true,
		},
		{
			name:          "conflicting port returns error",
			listen:        "0.0.0.0:8080",
			webhookListen: ":8080",
			wantErr:       true,
		},
		{
			name:          "invalid webhook address returns error",
			listen:        ":8080",
			webhookListen: "not-an-address",
			wantErr:       true,
		},
		{
			name:          "invalid port returns error",
			listen:        ":8080",
			webhookListen: ":not-a-port",
			wantErr:       true,
		},
		{
			name:          "port out of range returns error",
			listen:        ":8080",
			webhookListen: ":70000",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWebhookListen(tt.listen, tt.webhookListen)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateWebhookListen(%q, %q) err = %v, wantErr = %v", tt.listen, tt.webhookListen, err, tt.wantErr)
			}
		})
	}
}
