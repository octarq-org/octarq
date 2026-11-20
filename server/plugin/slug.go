package plugin

import (
	"crypto/rand"
)

const slugAlphabet = "abcdefghijkmnpqrstuvwxyz23456789"

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
