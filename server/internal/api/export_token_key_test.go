package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/config"
	"github.com/octarq-org/octarq/server/internal/crypto"
)

// Export tokens must be signed with the export subkey, not the raw master
// secret the session cookies and CSRF tokens also use.
func TestExportTokenUsesItsOwnSubkey(t *testing.T) {
	const secret = "master-secret"
	h := &Handler{cfg: &config.Config{SecretKey: secret}}

	tok, err := h.GenerateExportSignedToken(5, time.Minute)
	if err != nil {
		t.Fatalf("GenerateExportSignedToken: %v", err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil {
		t.Fatalf("decode token: %v", err)
	}
	parts := strings.Split(string(raw), ".")
	if len(parts) != 3 {
		t.Fatalf("want fileID.exp.sig, got %q", string(raw))
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		t.Fatalf("parse expiry: %v", err)
	}

	mac := hmac.New(sha256.New, crypto.DeriveKey([]byte(secret), "octarq/download-token/v1"))
	fmt.Fprintf(mac, "download:%s:%d", parts[0], exp)
	if want := hex.EncodeToString(mac.Sum(nil)); parts[2] != want {
		t.Fatalf("export token must use the download subkey: got %q want %q", parts[2], want)
	}

	rawMAC := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(rawMAC, "download:%s:%d", parts[0], exp)
	if parts[2] == hex.EncodeToString(rawMAC.Sum(nil)) {
		t.Fatal("export token must not be signed with the raw master secret")
	}
}
