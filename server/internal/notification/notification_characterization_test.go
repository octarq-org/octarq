package notification

import (
	"context"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

// TestCharacterization_RouterRegistration locks the Router channel registry contracts.
func TestCharacterization_RouterRegistration(t *testing.T) {
	db := setupTestDB(t)
	r := NewRouter(db)
	if r == nil {
		t.Fatal("NewRouter returned nil")
	}

	// Verify default registered channels
	emailCh, ok := r.GetChannel("email")
	if !ok || emailCh == nil {
		t.Fatal("expected email channel to be registered by default")
	}

	tgCh, ok := r.GetChannel("telegram")
	if !ok || tgCh == nil {
		t.Fatal("expected telegram channel to be registered by default")
	}

	inAppCh, ok := r.GetChannel("in_app")
	if !ok || inAppCh == nil {
		t.Fatal("expected in_app channel to be registered by default")
	}

	// Unknown channel must return false
	if _, ok := r.GetChannel("unknown_random_channel"); ok {
		t.Fatal("expected unknown channel lookup to return false")
	}
}

// TestCharacterization_SendDirectFailClosed locks fail-closed error responses
// when sending directly to invalid or malformed channels.
func TestCharacterization_SendDirectFailClosed(t *testing.T) {
	db := setupTestDB(t)
	r := NewRouter(db)

	ctx := context.Background()

	// 1. Unknown channel type must fail closed
	err := r.SendDirect(ctx, "non_existent_channel", "{}", "hello")
	if err == nil {
		t.Fatal("expected error for unknown channel type, got nil")
	}

	// 2. Malformed JSON config must fail closed with error (not silently swallowed)
	errMalformed := r.SendDirect(ctx, "email", "{bad_json:true", "hello")
	if errMalformed == nil {
		t.Fatal("expected error for malformed JSON config in SendDirect, got nil")
	}
}

// TestCharacterization_EventPatternMatching locks the event filter wildcards.
func TestCharacterization_EventPatternMatching(t *testing.T) {
	cases := []struct {
		pattern string
		event   string
		match   bool
	}{
		{"*", "security.login_failed", true},
		{"security.*", "security.login_failed", true},
		{"security.*", "cron.backup_done", false},
		{"*.job_failed", "cron.job_failed", true},
		{"*.job_failed", "cron.job_success", false},
		{"exact.event", "exact.event", true},
		{"exact.event", "other.event", false},
	}

	for _, tc := range cases {
		got := MatchEventPattern(tc.pattern, tc.event)
		if got != tc.match {
			t.Errorf("MatchEventPattern(%q, %q) = %v, want %v", tc.pattern, tc.event, got, tc.match)
		}
	}
}

// TestCharacterization_TenantDirectSecurity verifies that SendDirectWithTenant
// restricts channel credentials to the specified tenant.
func TestCharacterization_TenantDirectSecurity(t *testing.T) {
	db := setupTestDB(t)
	r := NewRouter(db)

	var interceptedOrg string
	mock := &mockChannel{
		name: "tenant_secure_channel",
		sendFunc: func(ctx context.Context, recipient plugin.NotificationRecipient, payload plugin.NotificationPayload) error {
			interceptedOrg = recipient.OrgID
			return nil
		},
	}
	_ = r.RegisterChannel(mock)

	// SendDirectWithTenant targeting org 42
	err := r.SendDirectWithTenant(context.Background(), 42, "tenant_secure_channel", "{}", "Alert text")
	if err != nil {
		t.Fatalf("SendDirectWithTenant failed: %v", err)
	}
	if interceptedOrg != "42" {
		t.Fatalf("expected intercepted recipient OrgID to be '42', got %q", interceptedOrg)
	}

	// SendDirect without context must have empty org
	interceptedOrg = ""
	err = r.SendDirect(context.Background(), "tenant_secure_channel", "{}", "Alert text")
	if err != nil {
		t.Fatalf("SendDirect failed: %v", err)
	}
	if interceptedOrg != "" {
		t.Fatalf("expected empty OrgID when no context is provided, got %q", interceptedOrg)
	}
}
