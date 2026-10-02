// Package token generates opaque credentials and their storage-safe digests.
package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

func New() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes)
}
func Hash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
