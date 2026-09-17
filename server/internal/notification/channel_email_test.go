package notification

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

func TestEmailChannel_Metadata(t *testing.T) {
	ch := NewEmailChannel(nil, nil)
	if ch.Name() != "email" {
		t.Errorf("got name %q, want email", ch.Name())
	}
	if ch.DisplayName() != "Email" {
		t.Errorf("got display name %q, want Email", ch.DisplayName())
	}
	schema := ch.ConfigSchema()
	if !strings.Contains(string(schema), "email") {
		t.Errorf("expected schema to mention email, got: %s", string(schema))
	}
}

func TestEmailChannel_SendSuccess(t *testing.T) {
	var sentOrgID uint
	var sentTo, sentSub, sentHTML, sentText string
	sender := func(ctx context.Context, orgID uint, to, subject, htmlBody, textBody string) error {
		sentOrgID = orgID
		sentTo = to
		sentSub = subject
		sentHTML = htmlBody
		sentText = textBody
		return nil
	}

	ch := NewEmailChannel(nil, nil)
	ch.SetSender(sender)

	ctx := context.Background()

	// 1. Send with recipient.Config["email"]
	rec := plugin.NotificationRecipient{
		OrgID: "42",
		Config: map[string]any{
			"email": "user@example.com",
		},
	}
	payload := plugin.NotificationPayload{
		Title: "Test Title",
		Body:  "Line 1\nLine 2",
	}
	if err := ch.Send(ctx, rec, payload); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if sentTo != "user@example.com" || sentOrgID != 42 || sentSub != "Test Title" {
		t.Errorf("unexpected send values: to=%q org=%d sub=%q", sentTo, sentOrgID, sentSub)
	}
	if !strings.Contains(sentHTML, "<br/>") || sentText != "Line 1\nLine 2" {
		t.Errorf("expected html with br and correct text body, got html=%s, text=%s", sentHTML, sentText)
	}

	// 2. Send with recipient.Config["to"] and default title
	recTo := plugin.NotificationRecipient{
		Config: map[string]any{
			"to":    "target@example.com",
			"orgId": float64(99),
		},
	}
	payloadNoTitle := plugin.NotificationPayload{
		EventType: "alert",
		Body:      "Body only",
	}
	if err := ch.Send(ctx, recTo, payloadNoTitle); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if sentTo != "target@example.com" || sentOrgID != 99 || sentSub != "[alert] Notification" {
		t.Errorf("unexpected send values: to=%q org=%d sub=%q", sentTo, sentOrgID, sentSub)
	}

	// 3. Send with org_id alternate key and fallback to 1
	recDefaultOrg := plugin.NotificationRecipient{
		Config: map[string]any{
			"email":  "test@example.com",
			"org_id": float64(77),
		},
	}
	if err := ch.Send(ctx, recDefaultOrg, payload); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if sentOrgID != 77 {
		t.Errorf("got orgID %d, want 77", sentOrgID)
	}
}

func TestEmailChannel_DBLookup(t *testing.T) {
	db := setupTestDB(t)
	user := models.User{Email: "dbuser@example.com"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	if err := db.Create(&models.OrgMember{UserID: user.ID, OrgID: 1, Role: "member"}).Error; err != nil {
		t.Fatalf("create org member failed: %v", err)
	}

	var sentTo string
	ch := NewEmailChannel(db, func(ctx context.Context, orgID uint, to, subject, htmlBody, textBody string) error {
		sentTo = to
		return nil
	})

	rec := plugin.NotificationRecipient{
		UserID: "1",
	}
	payload := plugin.NotificationPayload{
		Body: "Hello DB User",
	}
	if err := ch.Send(context.Background(), rec, payload); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if sentTo != "dbuser@example.com" {
		t.Errorf("got to=%q, want dbuser@example.com", sentTo)
	}
}

func TestEmailChannel_Errors(t *testing.T) {
	ch := NewEmailChannel(nil, nil)

	// Missing recipient email
	err := ch.Send(context.Background(), plugin.NotificationRecipient{}, plugin.NotificationPayload{})
	if err == nil || !strings.Contains(err.Error(), "cannot be resolved") {
		t.Errorf("expected cannot be resolved error, got: %v", err)
	}

	// Nil sender
	err = ch.Send(context.Background(), plugin.NotificationRecipient{
		OrgID:  "1",
		Config: map[string]any{"email": "valid@example.com"},
	}, plugin.NotificationPayload{})
	if err == nil || !strings.Contains(err.Error(), "no email sender configured") {
		t.Errorf("expected no sender error, got: %v", err)
	}

	// Missing or invalid orgID
	chWithSender := NewEmailChannel(nil, func(ctx context.Context, orgID uint, to, subject, htmlBody, textBody string) error {
		return nil
	})
	err = chWithSender.Send(context.Background(), plugin.NotificationRecipient{
		Config: map[string]any{"email": "valid@example.com"},
	}, plugin.NotificationPayload{})
	if err == nil || !strings.Contains(err.Error(), "missing or invalid tenant org context") {
		t.Errorf("expected missing org context error, got: %v", err)
	}

	// Sender returned error
	ch.SetSender(func(ctx context.Context, orgID uint, to, subject, htmlBody, textBody string) error {
		return errors.New("smtp timeout")
	})
	err = ch.Send(context.Background(), plugin.NotificationRecipient{
		OrgID:  "1",
		Config: map[string]any{"email": "valid@example.com"},
	}, plugin.NotificationPayload{})
	if err == nil || err.Error() != "smtp timeout" {
		t.Errorf("expected smtp timeout error, got: %v", err)
	}
}

func TestEmailChannel_TenantIsolationSpoofGuard(t *testing.T) {
	var capturedOrgID uint
	ch := NewEmailChannel(nil, func(ctx context.Context, orgID uint, to, subject, htmlBody, textBody string) error {
		capturedOrgID = orgID
		return nil
	})

	ctx := context.Background()
	payload := plugin.NotificationPayload{
		Title: "Security Isolation Test",
		Body:  "Testing anti-spoofing tenant isolation",
	}

	tests := []struct {
		name        string
		recipient   plugin.NotificationRecipient
		expectedOrg uint
		expectErr   bool
	}{
		{
			name: "Spoof attempt: legitimate system OrgID must not be overridden by config orgId",
			recipient: plugin.NotificationRecipient{
				OrgID: "42",
				Config: map[string]any{
					"email": "victim@example.com",
					"orgId": float64(999), // malicious attempt to hijack tenant 999 SMTP sender
				},
			},
			expectedOrg: 42,
		},
		{
			name: "Spoof attempt: legitimate system OrgID must not be overridden by config org_id",
			recipient: plugin.NotificationRecipient{
				OrgID: "42",
				Config: map[string]any{
					"email":  "victim@example.com",
					"org_id": float64(888), // malicious attempt to hijack tenant 888 SMTP sender
				},
			},
			expectedOrg: 42,
		},
		{
			name: "Fallback: empty system OrgID allows config orgId fallback",
			recipient: plugin.NotificationRecipient{
				OrgID: "",
				Config: map[string]any{
					"email": "user@example.com",
					"orgId": float64(100),
				},
			},
			expectedOrg: 100,
		},
		{
			name: "Fallback: invalid system OrgID allows config org_id fallback",
			recipient: plugin.NotificationRecipient{
				OrgID: "not-a-number",
				Config: map[string]any{
					"email":  "user@example.com",
					"org_id": float64(200),
				},
			},
			expectedOrg: 200,
		},
		{
			name: "Fallback: zero system OrgID allows config fallback",
			recipient: plugin.NotificationRecipient{
				OrgID: "0",
				Config: map[string]any{
					"email": "user@example.com",
					"orgId": float64(300),
				},
			},
			expectedOrg: 300,
		},
		{
			name: "Fail-closed: empty OrgID and empty config returns error",
			recipient: plugin.NotificationRecipient{
				OrgID: "",
				Config: map[string]any{
					"email": "user@example.com",
				},
			},
			expectErr: true,
		},
		{
			name: "Fail-closed: invalid OrgID and invalid config returns error",
			recipient: plugin.NotificationRecipient{
				OrgID: "bad_org",
				Config: map[string]any{
					"email": "user@example.com",
					"orgId": float64(-1),
				},
			},
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			capturedOrgID = 0
			err := ch.Send(ctx, tc.recipient, payload)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected fail-closed error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Send failed: %v", err)
			}
			if capturedOrgID != tc.expectedOrg {
				t.Errorf("tenant isolation spoof guard failed: got orgID %d, want %d", capturedOrgID, tc.expectedOrg)
			}
		})
	}
}
