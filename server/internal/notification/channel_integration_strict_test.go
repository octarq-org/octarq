package notification

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

func openChannelDB(t *testing.T, row *models.NotificationChannel) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.NotificationChannel{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	return db
}

func newWebhookRecorder(t *testing.T, target *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		*target = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func localSlackRow(t *testing.T) *models.NotificationChannel {
	t.Helper()
	return &models.NotificationChannel{
		OrgID: 1, Type: "slack", Enabled: true,
		Config: `{"webhookUrl":"https://hooks.slack.com/services/local-row"}`,
	}
}

func TestSlackChannel_NamedIntegrationNeverFallsBackToLocalRow(t *testing.T) {
	db := openChannelDB(t, localSlackRow(t))

	cases := []struct {
		name string
		host *plugin.TestHost
		want string
	}{
		{
			name: "hub has no such integration",
			host: &plugin.TestHost{LookupCredentialMock: func(context.Context, uint, plugin.CredentialQuery) (*plugin.Credential, error) {
				return nil, plugin.ErrCredentialNotFound
			}},
			want: "selected integration does not exist",
		},
		{
			name: "hub is not mounted",
			host: &plugin.TestHost{LookupCredentialMock: func(context.Context, uint, plugin.CredentialQuery) (*plugin.Credential, error) {
				return nil, plugin.ErrCredentialResolverUnavailable
			}},
			want: "selected integration does not exist",
		},
		{
			name: "hub failed",
			host: &plugin.TestHost{LookupCredentialMock: func(context.Context, uint, plugin.CredentialQuery) (*plugin.Credential, error) {
				return nil, errors.New("database is locked")
			}},
			want: "resolve integration credentials",
		},
		{
			name: "hub answered for another workspace",
			host: &plugin.TestHost{LookupCredentialMock: func(context.Context, uint, plugin.CredentialQuery) (*plugin.Credential, error) {
				return &plugin.Credential{ID: 7, OrgID: 2, ConfigJSON: json.RawMessage(`{"webhookUrl":"https://hooks.slack.com/services/other"}`)}, nil
			}},
			want: "selected integration does not exist",
		},
		{
			name: "integration has no webhook",
			host: &plugin.TestHost{LookupCredentialMock: func(context.Context, uint, plugin.CredentialQuery) (*plugin.Credential, error) {
				return &plugin.Credential{ID: 7, OrgID: 1}, nil
			}},
			want: "selected integration has no webhook url",
		},
	}
	for _, tc := range cases {
		ch := NewSlackChannel(db)
		ch.SetHost(tc.host)
		err := ch.Send(context.Background(), plugin.NotificationRecipient{
			OrgID: "1",
			Config: map[string]any{
				"integration_id": float64(7),
				"webhook_url":    "https://hooks.slack.com/services/stale-inline",
			},
		}, plugin.NotificationPayload{Body: "alert"})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: expected %q, got %v", tc.name, tc.want, err)
		}
	}
}

func TestSlackChannel_NamedIntegrationUsesHubCredential(t *testing.T) {
	db := openChannelDB(t, localSlackRow(t))

	var target string
	srv := newWebhookRecorder(t, &target)

	ch := NewSlackChannel(db)
	ch.SetHost(&plugin.TestHost{LookupCredentialMock: func(_ context.Context, orgID uint, q plugin.CredentialQuery) (*plugin.Credential, error) {
		if orgID != 1 || q.ProviderType != "communication" || q.Provider != "slack" || q.ID != 7 {
			t.Errorf("unexpected lookup %d %+v", orgID, q)
		}
		return &plugin.Credential{ID: 7, OrgID: 1, ConfigJSON: json.RawMessage(`{"webhook_url":"` + srv.URL + `"}`)}, nil
	}})
	ch.client = srv.Client()

	if err := ch.Send(context.Background(), plugin.NotificationRecipient{
		OrgID: "1",
		Config: map[string]any{
			"integration_id": float64(7),
			"webhook_url":    "https://hooks.slack.com/services/stale-inline",
		},
	}, plugin.NotificationPayload{Body: "alert"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if target == "" {
		t.Fatal("expected the hub webhook to receive the alert")
	}
}

func TestTelegramChannel_NamedIntegrationNeverFallsBackToLocalRow(t *testing.T) {
	db := openChannelDB(t, &models.NotificationChannel{
		OrgID: 1, Type: "telegram", Enabled: true,
		Config: `{"botToken":"local-token","chatId":"12345"}`,
	})

	cases := []struct {
		name string
		host *plugin.TestHost
		want string
	}{
		{
			name: "hub has no such integration",
			host: &plugin.TestHost{LookupCredentialMock: func(context.Context, uint, plugin.CredentialQuery) (*plugin.Credential, error) {
				return nil, plugin.ErrCredentialNotFound
			}},
			want: "selected integration does not exist",
		},
		{
			name: "hub is not mounted",
			host: &plugin.TestHost{LookupCredentialMock: func(context.Context, uint, plugin.CredentialQuery) (*plugin.Credential, error) {
				return nil, plugin.ErrCredentialResolverUnavailable
			}},
			want: "selected integration does not exist",
		},
		{
			name: "hub failed",
			host: &plugin.TestHost{LookupCredentialMock: func(context.Context, uint, plugin.CredentialQuery) (*plugin.Credential, error) {
				return nil, errors.New("database is locked")
			}},
			want: "resolve integration credentials",
		},
		{
			name: "hub answered for another workspace",
			host: &plugin.TestHost{LookupCredentialMock: func(context.Context, uint, plugin.CredentialQuery) (*plugin.Credential, error) {
				return &plugin.Credential{ID: 7, OrgID: 2, ConfigJSON: json.RawMessage(`{"botToken":"other"}`)}, nil
			}},
			want: "selected integration does not exist",
		},
		{
			name: "integration has no bot token",
			host: &plugin.TestHost{LookupCredentialMock: func(context.Context, uint, plugin.CredentialQuery) (*plugin.Credential, error) {
				return &plugin.Credential{ID: 7, OrgID: 1}, nil
			}},
			want: "selected integration has no bot token",
		},
	}
	for _, tc := range cases {
		ch := NewTelegramChannel(db)
		ch.SetHost(tc.host)
		err := ch.Send(context.Background(), plugin.NotificationRecipient{
			OrgID: "1",
			Config: map[string]any{
				"integrationId": 7,
				"chatId":        "12345",
				"botToken":      "stale-inline-token",
			},
		}, plugin.NotificationPayload{Body: "alert"})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: expected %q, got %v", tc.name, tc.want, err)
		}
	}
}

func TestTelegramChannel_ConfigDecryptFailureIsReported(t *testing.T) {
	db := openChannelDB(t, &models.NotificationChannel{
		OrgID: 1, Type: "telegram", Enabled: true, VaultItemID: 99, Config: `{"chatId":"12345"}`,
	})

	ch := NewTelegramChannel(db)
	ch.SetVaultService(&stubVaultService{err: errors.New("vault decrypt failed")})
	err := ch.Send(context.Background(), plugin.NotificationRecipient{OrgID: "1"}, plugin.NotificationPayload{Body: "alert"})
	if err == nil || !strings.Contains(err.Error(), "resolve channel config") {
		t.Fatalf("expected a resolve error, got %v", err)
	}
}
