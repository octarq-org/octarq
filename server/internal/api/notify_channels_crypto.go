package api

import (
	"encoding/json"
	"strings"
)

// channelConfigPlaintext returns the usable config JSON for a notification
// channel. Configs are AES-GCM encrypted at rest; a stored value that cannot be
// decrypted is an error rather than a passthrough of whatever was stored.
func (h *Handler) channelConfigPlaintext(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	b, err := h.cipher.Decrypt(stored)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// encryptChannelConfig seals a plaintext config JSON for storage.
func (h *Handler) encryptChannelConfig(plaintext string) (string, error) {
	return h.cipher.Encrypt([]byte(plaintext))
}

func redactConfigSecrets(cfgJSON string) string {
	if strings.TrimSpace(cfgJSON) == "" {
		return "{}"
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(cfgJSON), &m); err != nil {
		return cfgJSON
	}
	redactMap(m)
	b, err := json.Marshal(m)
	if err != nil {
		return cfgJSON
	}
	return string(b)
}

func redactMap(m map[string]any) {
	for k, v := range m {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "token") || strings.Contains(lk, "secret") || strings.Contains(lk, "password") ||
			strings.Contains(lk, "key") || strings.Contains(lk, "auth") || strings.Contains(lk, "credential") ||
			strings.Contains(lk, "bearer") {
			if s, ok := v.(string); ok && s != "" {
				m[k] = "[REDACTED]"
			}
		} else if child, ok := v.(map[string]any); ok {
			redactMap(child)
		}
	}
}

func mergeConfigPreservingSecrets(oldJSON, newJSON string) string {
	var oldMap, newMap map[string]any
	if json.Unmarshal([]byte(oldJSON), &oldMap) != nil {
		return newJSON
	}
	if json.Unmarshal([]byte(newJSON), &newMap) != nil {
		return newJSON
	}
	mergeMapsPreservingSecrets(oldMap, newMap)
	b, err := json.Marshal(newMap)
	if err != nil {
		return newJSON
	}
	return string(b)
}

func mergeMapsPreservingSecrets(oldMap, newMap map[string]any) {
	for k, v := range newMap {
		if s, ok := v.(string); ok && s == "[REDACTED]" {
			if oldVal, exists := oldMap[k]; exists {
				if oldStr, isStr := oldVal.(string); isStr && oldStr != "" {
					newMap[k] = oldStr
				}
			}
		} else if newChild, ok := v.(map[string]any); ok {
			if oldChild, exists := oldMap[k].(map[string]any); exists {
				mergeMapsPreservingSecrets(oldChild, newChild)
			}
		}
	}
}
