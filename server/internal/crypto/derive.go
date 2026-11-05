package crypto

import (
	"crypto/hkdf"
	"crypto/sha256"
	"fmt"
)

// Key purposes. Every consumer of the master secret names the job it uses the
// key for, so no two jobs share key material: a signature oracle in one place
// never yields the key another place trusts.
const (
	// PurposeCookieMAC signs session cookies (Cipher.Sign).
	PurposeCookieMAC = "octarq/cookie-mac/v1"
	// PurposeCSRF keys the double-submit CSRF token.
	PurposeCSRF = "octarq/csrf/v1"
	// PurposeTwoFAChallenge signs the pending OAuth second-factor challenge.
	PurposeTwoFAChallenge = "octarq/twofa-challenge/v1"
	// PurposeExportToken signs export download tokens.
	PurposeExportToken = "octarq/export-token/v1"
	// PurposeStorageURL keys presigned storage URLs.
	PurposeStorageURL = "octarq/storage-url/v1"
	// PurposeDownloadToken keys temporary file download tokens.
	PurposeDownloadToken = "octarq/download-token/v1"
	// PurposeOAuthState keys the OAuth state cookie store (HMAC and AES).
	PurposeOAuthState = "octarq/oauth-state/v1"
)

// DeriveKey returns the 32-byte subkey bound to one purpose, derived from the
// master secret with HKDF-SHA256. The purpose is the HKDF info parameter, so the
// same secret produces one unrelated key per label instead of one key reused
// everywhere.
//
// The inputs are constants (SHA-256 and a 32-byte length), so the only error
// HKDF can report is a programming mistake; it panics rather than returning a
// zero key that would silently weaken the caller.
func DeriveKey(secret []byte, purpose string) []byte {
	key, err := hkdf.Key(sha256.New, secret, nil, purpose, 32)
	if err != nil {
		panic(fmt.Sprintf("crypto: derive key %q: %v", purpose, err))
	}
	return key
}
