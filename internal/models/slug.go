package models

import "crypto/rand"

const slugAlphabet = "abcdefghijkmnpqrstuvwxyz23456789"

func RandomSlug(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	for i := range b {
		b[i] = slugAlphabet[int(b[i])%len(slugAlphabet)]
	}
	return string(b)
}
