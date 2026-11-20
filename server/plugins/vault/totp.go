package vault

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

func extractTOTPParams(payload map[string]any) (secret string, opts totp.ValidateOpts, err error) {
	opts = totp.ValidateOpts{
		Period:    30,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	}
	rawURI, _ := payload["otpauthUri"].(string)
	if rawURI == "" {
		rawURI, _ = payload["uri"].(string)
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(rawURI)), "otpauth://") {
		key, parseErr := otp.NewKeyFromURL(strings.TrimSpace(rawURI))
		if parseErr != nil {
			return "", opts, fmt.Errorf("invalid otpauth URI: %w", parseErr)
		}
		if key.Period() > 0 {
			opts.Period = uint(key.Period())
		}
		opts.Digits = key.Digits()
		opts.Algorithm = key.Algorithm()
		return key.Secret(), opts, nil
	}

	for _, k := range []string{"secret", "seed", "totpSecret"} {
		if v, ok := payload[k].(string); ok && strings.TrimSpace(v) != "" {
			secret = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(v), " ", ""))
			break
		}
	}
	if secret == "" {
		return "", opts, fmt.Errorf("TOTP secret or otpauthUri is required")
	}
	if strings.HasPrefix(strings.ToLower(secret), "OTPAUTH://") {
		if u, parseErr := url.Parse(secret); parseErr == nil && u.Query().Get("secret") != "" {
			secret = u.Query().Get("secret")
		}
	}
	if p, ok := payload["period"].(float64); ok && p >= 15 && p <= 300 {
		opts.Period = uint(p)
	}
	return secret, opts, nil
}

func computeTOTPCode(payload map[string]any, now time.Time) (string, int, error) {
	secret, opts, err := extractTOTPParams(payload)
	if err != nil {
		return "", 0, err
	}
	code, err := totp.GenerateCodeCustom(secret, now.UTC(), opts)
	if err != nil {
		return "", 0, fmt.Errorf("generate TOTP code: %w", err)
	}
	period := int(opts.Period)
	if period <= 0 {
		period = 30
	}
	remaining := period - int(now.UTC().Unix()%int64(period))
	return code, remaining, nil
}
