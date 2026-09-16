package notification

import (
	"errors"
	"sync"
)

var (
	decryptConfigMu sync.RWMutex
	decryptConfig   func(string) (string, bool)
)

// SetConfigDecryptor registers how a stored (encrypted) notification channel config
// is unwrapped before it is used for notification delivery.
func SetConfigDecryptor(fn func(string) (string, bool)) {
	decryptConfigMu.Lock()
	defer decryptConfigMu.Unlock()
	decryptConfig = fn
}

// ConfigPlaintext resolves the plaintext config for a stored value. A stored
// config that cannot be decrypted, or a build with no decryptor registered, is
// an error rather than a silent passthrough of whatever was stored.
func ConfigPlaintext(stored string) (string, error) {
	decryptConfigMu.RLock()
	fn := decryptConfig
	decryptConfigMu.RUnlock()
	if fn == nil {
		return "", errors.New("no config decryptor registered")
	}
	pt, ok := fn(stored)
	if !ok {
		return "", errors.New("stored notification channel config could not be decrypted")
	}
	return pt, nil
}
