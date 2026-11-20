package crypto

import (
	"crypto/hkdf"
	"crypto/sha256"
	"fmt"
)

const (
	PurposeCookieMAC      = "octarq/cookie-mac/v1"
	PurposeCSRF           = "octarq/csrf/v1"
	PurposeTwoFAChallenge = "octarq/twofa-challenge/v1"
	PurposeExportToken    = "octarq/export-token/v1"
	PurposeStorageURL     = "octarq/storage-url/v1"
	PurposeDownloadToken  = "octarq/download-token/v1"
	PurposeOAuthState     = "octarq/oauth-state/v1"
)

func DeriveKey(secret []byte, purpose string) []byte {
	key, err := hkdf.Key(sha256.New, secret, nil, purpose, 32)
	if err != nil {
		panic(fmt.Sprintf("crypto: derive key %q: %v", purpose, err))
	}
	return key
}
