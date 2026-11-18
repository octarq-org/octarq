package vault

import (
	"fmt"
	"strings"
)

// SchemaField describes one form field in a credential or vault item template.
type SchemaField struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Kind      string `json:"kind"` // text, secret, textarea, boolean, number, url
	Required  bool   `json:"required,omitempty"`
	Secret    bool   `json:"secret,omitempty"`
	Default   any    `json:"default,omitempty"`
	Hint      string `json:"hint,omitempty"`
	NonSecret bool   `json:"nonSecret,omitempty"` // also mirrored into searchable metadata_json
}

// ProviderSchema describes a typed provider template (for integrations or typed items).
type ProviderSchema struct {
	ID           string        `json:"id"`
	Label        string        `json:"label"`
	ItemType     string        `json:"itemType"`
	ProviderType string        `json:"providerType,omitempty"`
	Testable     bool          `json:"testable,omitempty"`
	Fields       []SchemaField `json:"fields"`
}

// CategorySchema groups related schemas for UI tabs and discovery.
type CategorySchema struct {
	Key       string           `json:"key"`
	Label     string           `json:"label"`
	ItemType  string           `json:"itemType"`
	Providers []ProviderSchema `json:"providers"`
}

// SchemasCatalog returns the authoritative schema catalog for all vault item types
// and third-party service integrations.
func SchemasCatalog() []CategorySchema {
	return []CategorySchema{
		{
			Key:      "llm",
			Label:    "AI / LLM Providers",
			ItemType: "integration",
			Providers: []ProviderSchema{
				{
					ID: "openai", Label: "OpenAI", ItemType: "integration", ProviderType: "llm", Testable: true,
					Fields: []SchemaField{
						{Key: "apiKey", Label: "API Key", Kind: "secret", Required: true, Secret: true},
						{Key: "model", Label: "Default Model", Kind: "text", NonSecret: true},
						{Key: "cheapModel", Label: "Cheap Model", Kind: "text", NonSecret: true},
						{Key: "baseUrl", Label: "Base URL", Kind: "url", NonSecret: true},
					},
				},
				{
					ID: "anthropic", Label: "Anthropic Claude", ItemType: "integration", ProviderType: "llm", Testable: true,
					Fields: []SchemaField{
						{Key: "apiKey", Label: "API Key", Kind: "secret", Required: true, Secret: true},
						{Key: "model", Label: "Default Model", Kind: "text", NonSecret: true},
						{Key: "baseUrl", Label: "Base URL", Kind: "url", NonSecret: true},
					},
				},
				{
					ID: "deepseek", Label: "DeepSeek", ItemType: "integration", ProviderType: "llm", Testable: true,
					Fields: []SchemaField{
						{Key: "apiKey", Label: "API Key", Kind: "secret", Required: true, Secret: true},
						{Key: "model", Label: "Default Model", Kind: "text", NonSecret: true},
						{Key: "baseUrl", Label: "Base URL", Kind: "url", NonSecret: true},
					},
				},
				{
					ID: "gemini", Label: "Google Gemini", ItemType: "integration", ProviderType: "llm", Testable: true,
					Fields: []SchemaField{
						{Key: "apiKey", Label: "API Key", Kind: "secret", Required: true, Secret: true},
						{Key: "model", Label: "Default Model", Kind: "text", NonSecret: true},
					},
				},
				{
					ID: "ollama", Label: "Local / Ollama", ItemType: "integration", ProviderType: "llm", Testable: true,
					Fields: []SchemaField{
						{Key: "baseUrl", Label: "Base URL", Kind: "url", Required: true, NonSecret: true},
						{Key: "model", Label: "Default Model", Kind: "text", NonSecret: true},
						{Key: "apiKey", Label: "API Key (Optional)", Kind: "secret", Secret: true},
					},
				},
			},
		},
		{
			Key:      "payment",
			Label:    "Payment Rails",
			ItemType: "integration",
			Providers: []ProviderSchema{
				{
					ID: "stripe", Label: "Stripe", ItemType: "integration", ProviderType: "payment", Testable: true,
					Fields: []SchemaField{
						{Key: "secretKey", Label: "Secret Key", Kind: "secret", Required: true, Secret: true},
						{Key: "webhookSecret", Label: "Webhook Secret", Kind: "secret", Secret: true},
					},
				},
				{
					ID: "polar", Label: "Polar", ItemType: "integration", ProviderType: "payment", Testable: true,
					Fields: []SchemaField{
						{Key: "secretKey", Label: "Access Token", Kind: "secret", Secret: true},
						{Key: "webhookSecret", Label: "Webhook Secret", Kind: "secret", Required: true, Secret: true},
					},
				},
				{
					ID: "wechat", Label: "WeChat Pay v3", ItemType: "integration", ProviderType: "payment", Testable: true,
					Fields: []SchemaField{
						{Key: "mchId", Label: "Merchant ID", Kind: "text", Required: true, NonSecret: true},
						{Key: "appId", Label: "App ID", Kind: "text", NonSecret: true},
						{Key: "secretKey", Label: "API v3 Key", Kind: "secret", Required: true, Secret: true},
						{Key: "serialNo", Label: "Certificate Serial No", Kind: "text", Required: true, NonSecret: true},
						{Key: "privateKey", Label: "Merchant Private Key (PEM)", Kind: "textarea", Required: true, Secret: true},
					},
				},
				{
					ID: "alipay", Label: "Alipay", ItemType: "integration", ProviderType: "payment", Testable: true,
					Fields: []SchemaField{
						{Key: "appId", Label: "App ID", Kind: "text", Required: true, NonSecret: true},
						{Key: "secretKey", Label: "App Private Key", Kind: "textarea", Required: true, Secret: true},
						{Key: "alipayPublicKey", Label: "Alipay Public Key", Kind: "textarea", Required: true, NonSecret: true},
						{Key: "gatewayUrl", Label: "Gateway URL", Kind: "url", NonSecret: true},
					},
				},
			},
		},
		{
			Key:      "storage",
			Label:    "Object Storage",
			ItemType: "integration",
			Providers: []ProviderSchema{
				{
					ID: "s3", Label: "AWS S3", ItemType: "integration", ProviderType: "storage", Testable: true,
					Fields: []SchemaField{
						{Key: "bucket", Label: "Bucket Name", Kind: "text", Required: true, NonSecret: true},
						{Key: "region", Label: "Region", Kind: "text", NonSecret: true},
						{Key: "baseUrl", Label: "Endpoint URL (Optional)", Kind: "url", NonSecret: true},
						{Key: "apiKey", Label: "Access Key ID", Kind: "text", Required: true, NonSecret: true},
						{Key: "secretKey", Label: "Secret Access Key", Kind: "secret", Required: true, Secret: true},
					},
				},
				{
					ID: "r2", Label: "Cloudflare R2", ItemType: "integration", ProviderType: "storage", Testable: true,
					Fields: []SchemaField{
						{Key: "bucket", Label: "Bucket Name", Kind: "text", Required: true, NonSecret: true},
						{Key: "baseUrl", Label: "Endpoint URL", Kind: "url", Required: true, NonSecret: true},
						{Key: "apiKey", Label: "Access Key ID", Kind: "text", Required: true, NonSecret: true},
						{Key: "secretKey", Label: "Secret Access Key", Kind: "secret", Required: true, Secret: true},
					},
				},
				{
					ID: "minio", Label: "MinIO", ItemType: "integration", ProviderType: "storage", Testable: true,
					Fields: []SchemaField{
						{Key: "bucket", Label: "Bucket Name", Kind: "text", Required: true, NonSecret: true},
						{Key: "baseUrl", Label: "Endpoint URL", Kind: "url", Required: true, NonSecret: true},
						{Key: "apiKey", Label: "Access Key", Kind: "text", Required: true, NonSecret: true},
						{Key: "secretKey", Label: "Secret Key", Kind: "secret", Required: true, Secret: true},
						{Key: "pathStyle", Label: "Path-style URLs", Kind: "boolean", Default: true, NonSecret: true},
					},
				},
			},
		},
		{
			Key:      "communication",
			Label:    "Communication Channels",
			ItemType: "integration",
			Providers: []ProviderSchema{
				{
					ID: "telegram", Label: "Telegram Bot", ItemType: "integration", ProviderType: "communication", Testable: true,
					Fields: []SchemaField{
						{Key: "botToken", Label: "Bot Token", Kind: "secret", Required: true, Secret: true},
						{Key: "chatId", Label: "Default Chat ID", Kind: "text", NonSecret: true},
					},
				},
				{
					ID: "slack", Label: "Slack App", ItemType: "integration", ProviderType: "communication", Testable: true,
					Fields: []SchemaField{
						{Key: "botToken", Label: "Bot Token", Kind: "secret", Secret: true},
						{Key: "webhookUrl", Label: "Webhook URL", Kind: "secret", Secret: true},
						{Key: "webhookSecret", Label: "Signing Secret", Kind: "secret", Secret: true},
					},
				},
				{
					ID: "resend", Label: "Resend", ItemType: "integration", ProviderType: "communication", Testable: true,
					Fields: []SchemaField{
						{Key: "apiKey", Label: "API Key", Kind: "secret", Required: true, Secret: true},
						{Key: "fromEmail", Label: "From Email", Kind: "text", NonSecret: true},
					},
				},
				{
					ID: "smtp", Label: "SMTP Service", ItemType: "integration", ProviderType: "communication", Testable: true,
					Fields: []SchemaField{
						{Key: "smtpHost", Label: "SMTP Host", Kind: "text", Required: true, NonSecret: true},
						{Key: "smtpPort", Label: "SMTP Port", Kind: "number", Default: 587, NonSecret: true},
						{Key: "apiKey", Label: "Username", Kind: "text", NonSecret: true},
						{Key: "secretKey", Label: "Password", Kind: "secret", Required: true, Secret: true},
						{Key: "tlsMode", Label: "TLS Mode", Kind: "text", Default: "starttls", NonSecret: true},
					},
				},
			},
		},
		{
			Key:      "dns",
			Label:    "DNS & Domains",
			ItemType: "integration",
			Providers: []ProviderSchema{
				{
					ID: "cloudflare", Label: "Cloudflare", ItemType: "integration", ProviderType: "dns", Testable: true,
					Fields: []SchemaField{
						{Key: "apiToken", Label: "API Token", Kind: "secret", Required: true, Secret: true},
						{Key: "zoneId", Label: "Zone ID (Optional)", Kind: "text", NonSecret: true},
					},
				},
				{
					ID: "dnspod", Label: "Tencent DNSPod", ItemType: "integration", ProviderType: "dns", Testable: true,
					Fields: []SchemaField{
						{Key: "secretId", Label: "Secret ID", Kind: "text", Required: true, NonSecret: true},
						{Key: "secretKey", Label: "Secret Key", Kind: "secret", Required: true, Secret: true},
					},
				},
				{
					ID: "route53", Label: "AWS Route53", ItemType: "integration", ProviderType: "dns", Testable: true,
					Fields: []SchemaField{
						{Key: "region", Label: "Region", Kind: "text", NonSecret: true},
						{Key: "apiKey", Label: "Access Key ID", Kind: "text", Required: true, NonSecret: true},
						{Key: "secretKey", Label: "Secret Access Key", Kind: "secret", Required: true, Secret: true},
					},
				},
			},
		},
		{
			Key:      "secrets",
			Label:    "Secrets & Infrastructure",
			ItemType: "kv_secret",
			Providers: []ProviderSchema{
				{
					ID: "kv_secret", Label: "Key-Value Secret", ItemType: "kv_secret",
					Fields: []SchemaField{
						{Key: "value", Label: "Secret Value", Kind: "secret", Required: true, Secret: true},
					},
				},
				{
					ID: "api_token", Label: "API Token", ItemType: "api_token",
					Fields: []SchemaField{
						{Key: "token", Label: "Token", Kind: "secret", Required: true, Secret: true},
						{Key: "endpoint", Label: "Endpoint / Service", Kind: "url", NonSecret: true},
					},
				},
				{
					ID: "ssh_key", Label: "SSH Private Key", ItemType: "ssh_key", Testable: true,
					Fields: []SchemaField{
						{Key: "privateKey", Label: "Private Key (PEM)", Kind: "textarea", Required: true, Secret: true},
						{Key: "publicKey", Label: "Public Key", Kind: "textarea", NonSecret: true},
						{Key: "passphrase", Label: "Passphrase (Optional)", Kind: "secret", Secret: true},
					},
				},
				{
					ID: "database", Label: "Database Credential", ItemType: "database", Testable: true,
					Fields: []SchemaField{
						{Key: "driver", Label: "Driver", Kind: "text", Default: "postgres", NonSecret: true},
						{Key: "host", Label: "Host", Kind: "text", Required: true, NonSecret: true},
						{Key: "port", Label: "Port", Kind: "number", Default: 5432, NonSecret: true},
						{Key: "database", Label: "Database", Kind: "text", NonSecret: true},
						{Key: "username", Label: "Username", Kind: "text", NonSecret: true},
						{Key: "password", Label: "Password", Kind: "secret", Required: true, Secret: true},
					},
				},
				{
					ID: "certificate", Label: "TLS Certificate", ItemType: "certificate", Testable: true,
					Fields: []SchemaField{
						{Key: "certPem", Label: "Certificate PEM", Kind: "textarea", Required: true, NonSecret: true},
						{Key: "keyPem", Label: "Private Key PEM", Kind: "textarea", Required: true, Secret: true},
						{Key: "chainPem", Label: "Chain PEM", Kind: "textarea", NonSecret: true},
					},
				},
				{
					ID: "oauth_app", Label: "OAuth / OIDC Client", ItemType: "oauth_app", ProviderType: "sso",
					Fields: []SchemaField{
						{Key: "issuer", Label: "Issuer URL", Kind: "url", NonSecret: true},
						{Key: "clientId", Label: "Client ID", Kind: "text", Required: true, NonSecret: true},
						{Key: "clientSecret", Label: "Client Secret", Kind: "secret", Required: true, Secret: true},
					},
				},
				{
					ID: "webhook", Label: "Webhook Signing Secret", ItemType: "webhook",
					Fields: []SchemaField{
						{Key: "signingSecret", Label: "Signing Secret", Kind: "secret", Required: true, Secret: true},
						{Key: "targetUrl", Label: "Target URL", Kind: "url", NonSecret: true},
					},
				},
				{
					ID: "signing_key", Label: "Ed25519 Signing Key", ItemType: "signing_key",
					Fields: []SchemaField{
						{Key: "privateKey", Label: "Private Key", Kind: "textarea", Required: true, Secret: true},
						{Key: "publicKey", Label: "Public Key", Kind: "text", NonSecret: true},
					},
				},
			},
		},
		{
			Key:      "vault_items",
			Label:    "Logins, 2FA & Notes",
			ItemType: "login",
			Providers: []ProviderSchema{
				{
					ID: "login", Label: "Login / Account", ItemType: "login",
					Fields: []SchemaField{
						{Key: "url", Label: "Website URL", Kind: "url", NonSecret: true},
						{Key: "username", Label: "Username / Email", Kind: "text", Required: true, NonSecret: true},
						{Key: "password", Label: "Password", Kind: "secret", Required: true, Secret: true},
						{Key: "otpauthUri", Label: "2FA TOTP Seed / URI", Kind: "secret", Secret: true},
						{Key: "notes", Label: "Notes", Kind: "textarea"},
					},
				},
				{
					ID: "twofa_totp", Label: "2FA Authenticator (TOTP)", ItemType: "twofa_totp", Testable: true,
					Fields: []SchemaField{
						{Key: "issuer", Label: "Issuer", Kind: "text", NonSecret: true},
						{Key: "account", Label: "Account", Kind: "text", NonSecret: true},
						{Key: "secret", Label: "TOTP Secret (Base32)", Kind: "secret", Secret: true},
						{Key: "otpauthUri", Label: "otpauth:// URI (Optional)", Kind: "secret", Secret: true},
					},
				},
				{
					ID: "tracker_credential", Label: "Tracker Credential", ItemType: "tracker_credential", ProviderType: "tracker",
					Fields: []SchemaField{
						{Key: "login", Label: "Login Secret", Kind: "secret", Secret: true},
						{Key: "cookie", Label: "Cookie", Kind: "textarea", Secret: true},
						{Key: "site_api_token", Label: "Site API Token", Kind: "secret", Secret: true},
						{Key: "token", Label: "Token", Kind: "secret", Secret: true},
					},
				},
				{
					ID: "secure_note", Label: "Secure Note", ItemType: "secure_note",
					Fields: []SchemaField{
						{Key: "content", Label: "Note Content", Kind: "textarea", Required: true, Secret: true},
					},
				},
			},
		},
	}
}

func findProviderSchema(itemType, providerType, provider string) *ProviderSchema {
	pLower := strings.ToLower(strings.TrimSpace(provider))
	ptLower := strings.ToLower(strings.TrimSpace(providerType))
	itLower := strings.ToLower(strings.TrimSpace(itemType))
	for _, cat := range SchemasCatalog() {
		for i := range cat.Providers {
			ps := cat.Providers[i]
			if pLower != "" && ps.ID == pLower && (ptLower == "" || ps.ProviderType == ptLower) {
				return &ps
			}
			if pLower == "" && itLower != "" && ps.ID == itLower {
				return &ps
			}
		}
	}
	return nil
}

func validatePayload(itemType, providerType, provider string, payload map[string]any) error {
	if len(payload) == 0 {
		return fmt.Errorf("payload cannot be empty")
	}
	ps := findProviderSchema(itemType, providerType, provider)
	if ps == nil {
		return nil
	}
	if ps.ID == "twofa_totp" {
		if _, _, err := extractTOTPParams(payload); err != nil {
			return err
		}
		return nil
	}
	for _, f := range ps.Fields {
		if !f.Required {
			continue
		}
		v, ok := payload[f.Key]
		if !ok || v == nil {
			return fmt.Errorf("field %q is required for %s", f.Key, ps.Label)
		}
		if s, isStr := v.(string); isStr && strings.TrimSpace(s) == "" {
			return fmt.Errorf("field %q cannot be blank for %s", f.Key, ps.Label)
		}
	}
	return nil
}

func extractNonSecretMetadata(itemType, providerType, provider string, payload, extraMeta map[string]any) map[string]any {
	meta := map[string]any{}
	for k, v := range extraMeta {
		if !looksSecretKey(k) {
			meta[k] = v
		}
	}
	if ps := findProviderSchema(itemType, providerType, provider); ps != nil {
		for _, f := range ps.Fields {
			if f.NonSecret && !f.Secret {
				if v, ok := payload[f.Key]; ok && v != nil && fmt.Sprint(v) != "" {
					meta[f.Key] = v
				}
			}
		}
	}
	return meta
}

func looksSecretKey(k string) bool {
	lk := strings.ToLower(k)
	for _, marker := range []string{"secret", "password", "token", "private", "apikey", "api_key", "seed", "cookie", "passphrase", "credential"} {
		if strings.Contains(lk, marker) {
			return true
		}
	}
	return false
}

func extractFieldSchema(payload map[string]any, itemType, providerType, provider string) map[string]string {
	out := map[string]string{}
	ps := findProviderSchema(itemType, providerType, provider)
	secretByKey := map[string]bool{}
	if ps != nil {
		for _, f := range ps.Fields {
			secretByKey[f.Key] = f.Secret
		}
	}
	for k := range payload {
		if secret, ok := secretByKey[k]; ok {
			if secret {
				out[k] = "secret"
			} else {
				out[k] = "text"
			}
			continue
		}
		if looksSecretKey(k) {
			out[k] = "secret"
		} else {
			out[k] = "text"
		}
	}
	return out
}
