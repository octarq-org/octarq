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

// openChannelDB seeds one local notification channel row and returns the DB.
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

// newWebhookRecorder returns a server that records the last message body it
// received, so a test can prove which destination was used.
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

// A recipient that names an integration is answered by the Hub alone. These
// tests pin that a missing or broken integration never falls through to the
// local notification_channels row: delivering through a credential the operator
// did not select is worse than not delivering at all.

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
			// The stale inline URL must not be used: the named integration owns
			// the destination.
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
			// The stale inline token must not be used either.
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

// TestTelegramChannel_ConfigDecryptFailureIsReported pins that an unreadable
// local row is an error, not a silent "no credentials configured".
func TestTelegramChannel_ConfigDecryptFailureIsReported(t *testing.T) {
	db := openChannelDB(t, &models.NotificationChannel{
		OrgID: 1, Type: "telegram", Enabled: true, Config: "corrupted",
	})
	SetConfigDecryptor(func(string) (string, bool) { return "", false })
	t.Cleanup(func() { SetConfigDecryptor(nil) })

	ch := NewTelegramChannel(db)
	err := ch.Send(context.Background(), plugin.NotificationRecipient{OrgID: "1"}, plugin.NotificationPayload{Body: "alert"})
	if err == nil || !strings.Contains(err.Error(), "decrypt channel config") {
		t.Fatalf("expected a decrypt error, got %v", err)
	}
}
