package plugin

import (
	"crypto/rand"
)

const slugAlphabet = "abcdefghijkmnpqrstuvwxyz23456789"

// RandomSlug generates a cryptographically secure random alphanumeric slug of length n
// using an unconfusable base32 alphabet (no 0, 1, l, o).
func RandomSlug(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		panic("plugin: crypto/rand entropy failure: " + err.Error())
	}
	for i := range b {
		b[i] = slugAlphabet[int(b[i])%len(slugAlphabet)]
	}
	return string(b)
}
