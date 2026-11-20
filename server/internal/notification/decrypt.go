package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

func ResolveChannelConfig(ctx context.Context, vs plugin.VaultService, ch models.NotificationChannel) (map[string]any, error) {
	cfg := make(map[string]any)
	if raw := strings.TrimSpace(ch.Config); raw != "" && raw != "{}" {
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return nil, fmt.Errorf("invalid notification channel config JSON: %w", err)
		}
	}
	if ch.VaultItemID > 0 {
		if vs == nil {
			return nil, errors.New("no vault service registered")
		}
		sec, err := vs.GetSecret(ctx, ch.OrgID, ch.VaultItemID)
		if err != nil {
			return nil, fmt.Errorf("resolve notification channel secret: %w", err)
		}
		for k, v := range sec.Payload {
			cfg[k] = v
		}
	}
	return cfg, nil
}

func ResolveChannelConfigJSON(ctx context.Context, vs plugin.VaultService, ch models.NotificationChannel) (string, error) {
	cfg, err := ResolveChannelConfig(ctx, vs, ch)
	if err != nil {
		return "", err
	}
	if len(cfg) == 0 {
		return "{}", nil
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
