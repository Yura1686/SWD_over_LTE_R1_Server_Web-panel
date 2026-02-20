package util

import (
	"crypto/rand"
	"encoding/hex"
)

// RandomToken returns random hex token with prefix.
func RandomToken(prefix string, randomBytes int) string {
	buf := make([]byte, randomBytes)
	_, err := rand.Read(buf)
	if err != nil {
		return prefix + "_fallback"
	}
	return prefix + "_" + hex.EncodeToString(buf)
}
