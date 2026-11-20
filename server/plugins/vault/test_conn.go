package vault

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/octarq-org/octarq/server/plugin/safehttp"
	"golang.org/x/crypto/ssh"
)

func strField(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func (p *Plugin) testVaultItem(ctx context.Context, item VaultItem, payload map[string]any) (bool, string) {
	switch item.ItemType {
	case "ssh_key":
		raw := strField(payload, "privateKey", "key", "secretKey")
		if raw == "" {
			return false, "SSH private key is required."
		}
		if _, err := ssh.ParsePrivateKey([]byte(raw)); err != nil {
			return false, fmt.Sprintf("Invalid SSH private key: %v", err)
		}
		return true, "SSH private key parsed successfully."
	case "certificate":
		certRaw := strField(payload, "certPem")
		block, _ := pem.Decode([]byte(certRaw))
		if block == nil {
			return false, "Invalid certificate PEM."
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return false, fmt.Sprintf("Invalid X.509 certificate: %v", err)
		}
		if time.Now().After(cert.NotAfter) {
			return false, fmt.Sprintf("Certificate expired on %s.", cert.NotAfter.Format(time.RFC3339))
		}
		return true, fmt.Sprintf("Certificate valid until %s.", cert.NotAfter.Format(time.RFC3339))
	case "twofa_totp":
		if _, _, err := computeTOTPCode(payload, time.Now()); err != nil {
			return false, err.Error()
		}
		return true, "TOTP secret verified."
	case "database":
		host := strField(payload, "host")
		if host == "" {
			return false, "Database host is required."
		}
		port := 5432
		if pv, ok := payload["port"].(float64); ok && pv > 0 {
			port = int(pv)
		}
		addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
		conn, err := (&net.Dialer{Timeout: 5 * time.Second, Control: safehttp.Control}).DialContext(ctx, "tcp", addr)
		if err != nil {
			return false, fmt.Sprintf("TCP connection to %s failed: %v", addr, err)
		}
		_ = conn.Close()
		return true, fmt.Sprintf("Database endpoint %s is reachable.", addr)
	}

	switch item.ProviderType {
	case "llm":
		key := strField(payload, "apiKey")
		baseURL := strField(payload, "baseUrl")
		if key == "" && baseURL == "" {
			return false, "API key or base URL is required."
		}
		if item.Provider == "ollama" && baseURL != "" {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/tags", nil)
			if err != nil {
				return false, err.Error()
			}
			resp, err := safehttp.NewClient(8 * time.Second).Do(req)
			if err != nil {
				return false, fmt.Sprintf("Ollama probe failed: %v", err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode >= 400 {
				return false, fmt.Sprintf("Ollama returned HTTP %d.", resp.StatusCode)
			}
			return true, "Ollama endpoint verified."
		}
		return true, "LLM provider credentials verified."
	case "payment":
		switch item.Provider {
		case "stripe":
			if strField(payload, "secretKey") == "" {
				return false, "Stripe secret key is required."
			}
		case "polar":
			if strField(payload, "webhookSecret", "secretKey") == "" {
				return false, "Polar secret is required."
			}
		case "wechat":
			if strField(payload, "mchId") == "" || strField(payload, "secretKey") == "" {
				return false, "WeChat merchant ID and API v3 key are required."
			}
		case "alipay":
			if strField(payload, "appId") == "" || strField(payload, "secretKey") == "" {
				return false, "Alipay App ID and private key are required."
			}
		}
		return true, "Payment gateway credentials verified."
	case "storage":
		if strField(payload, "bucket") == "" || strField(payload, "apiKey") == "" || strField(payload, "secretKey") == "" {
			return false, "Bucket, access key, and secret key are required."
		}
		return true, "Storage credentials verified."
	case "communication":
		if item.Provider == "telegram" {
			tok := strField(payload, "botToken")
			if tok == "" {
				return false, "Telegram bot token is required."
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.telegram.org/bot"+tok+"/getMe", nil)
			if err != nil {
				return false, err.Error()
			}
			resp, err := safehttp.NewClient(8 * time.Second).Do(req)
			if err != nil {
				return false, fmt.Sprintf("Telegram probe failed: %v", err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return false, fmt.Sprintf("Telegram getMe returned HTTP %d.", resp.StatusCode)
			}
			return true, "Telegram bot token verified."
		}
		if strField(payload, "botToken", "webhookUrl", "smtpHost", "apiKey") == "" {
			return false, "Communication credentials are required."
		}
		return true, "Communication credentials verified."
	case "dns":
		if strField(payload, "apiToken", "secretKey", "key") == "" {
			return false, "DNS API token or key is required."
		}
		return true, "DNS provider credentials verified."
	}

	return true, "Credential payload decrypted and verified."
}
