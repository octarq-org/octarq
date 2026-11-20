package api

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

func isSecretConfigKey(k string) bool {
	lk := strings.ToLower(k)
	for _, marker := range []string{"token", "secret", "password", "key", "auth", "credential", "bearer", "webhookurl", "webhook_url"} {
		if strings.Contains(lk, marker) {
			return true
		}
	}
	return false
}

func splitChannelConfig(cfgJSON string) (metaMap map[string]any, secretMap map[string]any, redactedKeys []string, err error) {
	metaMap = make(map[string]any)
	secretMap = make(map[string]any)
	trimmed := strings.TrimSpace(cfgJSON)
	if trimmed == "" || trimmed == "{}" {
		return metaMap, secretMap, nil, nil
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return nil, nil, nil, err
	}
	for k, v := range raw {
		if isSecretConfigKey(k) {
			if s, ok := v.(string); ok {
				if s == "[REDACTED]" {
					redactedKeys = append(redactedKeys, k)
					continue
				}
				if strings.TrimSpace(s) == "" {
					continue
				}
			} else if v == nil {
				continue
			}
			secretMap[k] = v
		} else {
			metaMap[k] = v
		}
	}
	return metaMap, secretMap, redactedKeys, nil
}

func marshalChannelConfigMap(m map[string]any) string {
	if len(m) == 0 {
		return "{}"
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func buildChannelConfigView(ctx context.Context, vs plugin.VaultService, ch models.NotificationChannel) string {
	viewMap := make(map[string]any)
	if raw := strings.TrimSpace(ch.Config); raw != "" && raw != "{}" {
		_ = json.Unmarshal([]byte(raw), &viewMap)
	}
	if ch.VaultItemID > 0 && vs != nil {
		if meta, err := vs.GetMeta(ctx, ch.OrgID, ch.VaultItemID); err == nil && meta != nil {
			for _, fk := range meta.FieldKeys {
				viewMap[fk] = "[REDACTED]"
			}
		}
	}
	return marshalChannelConfigMap(viewMap)
}
