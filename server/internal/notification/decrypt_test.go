package notification

import (
	"context"
	"errors"
	"testing"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

type stubVaultService struct {
	secrets map[uint]map[string]any
	err     error
}

func (s *stubVaultService) PutSecret(context.Context, plugin.PutSecretRequest) (*plugin.VaultItemMeta, error) {
	return nil, nil
}
func (s *stubVaultService) UpdateSecret(context.Context, uint, uint, *string, map[string]any, map[string]any, *bool, string, uint) (*plugin.VaultItemMeta, error) {
	return nil, nil
}
func (s *stubVaultService) GetMeta(context.Context, uint, uint) (*plugin.VaultItemMeta, error) {
	return nil, nil
}
func (s *stubVaultService) GetSecret(_ context.Context, orgID, itemID uint) (*plugin.VaultSecret, error) {
	if s.err != nil {
		return nil, s.err
	}
	payload, ok := s.secrets[itemID]
	if !ok {
		return nil, plugin.ErrSecretNotFound
	}
	return &plugin.VaultSecret{
		Meta:    plugin.VaultItemMeta{ID: itemID, OrgID: orgID},
		Payload: payload,
	}, nil
}
func (s *stubVaultService) ResolveByRef(context.Context, uint, string) (*plugin.VaultSecret, error) {
	return nil, nil
}
func (s *stubVaultService) ResolveInstanceSecret(context.Context, string) (*plugin.VaultSecret, error) {
	return nil, nil
}
func (s *stubVaultService) PutInstanceSecret(context.Context, plugin.PutSecretRequest) (*plugin.VaultItemMeta, error) {
	return nil, nil
}
func (s *stubVaultService) DeleteInstanceSecret(context.Context, string) error { return nil }
func (s *stubVaultService) ListItems(context.Context, uint, plugin.VaultFilter) ([]plugin.VaultItemMeta, error) {
	return nil, nil
}
func (s *stubVaultService) DeleteSecret(context.Context, uint, uint) error { return nil }
func (s *stubVaultService) ComputeTOTP(context.Context, uint, uint) (string, int, error) {
	return "", 0, nil
}

func TestResolveChannelConfig(t *testing.T) {
	ctx := context.Background()

	// 1. Metadata-only channel without VaultItemID
	cfg, err := ResolveChannelConfig(ctx, nil, models.NotificationChannel{
		OrgID:  1,
		Config: `{"chatId":"42"}`,
	})
	if err != nil || cfg["chatId"] != "42" {
		t.Fatalf("metadata-only channel failed: cfg=%v err=%v", cfg, err)
	}

	// 2. Missing VaultService when VaultItemID > 0 fails closed
	if _, err := ResolveChannelConfig(ctx, nil, models.NotificationChannel{
		OrgID:       1,
		VaultItemID: 10,
		Config:      `{"chatId":"42"}`,
	}); err == nil {
		t.Error("expected error when vault service is nil for VaultItemID > 0")
	}

	// 3. VaultService error fails closed
	failing := &stubVaultService{err: errors.New("vault unavailable")}
	if _, err := ResolveChannelConfig(ctx, failing, models.NotificationChannel{
		OrgID:       1,
		VaultItemID: 10,
	}); err == nil {
		t.Error("expected error when vault GetSecret fails")
	}

	// 4. Metadata + Vault payload merge cleanly
	vs := &stubVaultService{secrets: map[uint]map[string]any{
		10: {"botToken": "secret-tok"},
	}}
	cfgJSON, err := ResolveChannelConfigJSON(ctx, vs, models.NotificationChannel{
		OrgID:       1,
		VaultItemID: 10,
		Config:      `{"chatId":"42"}`,
	})
	if err != nil {
		t.Fatalf("ResolveChannelConfigJSON failed: %v", err)
	}
	if cfgJSON == "" || cfgJSON == "{}" {
		t.Fatalf("unexpected empty JSON: %q", cfgJSON)
	}
}
