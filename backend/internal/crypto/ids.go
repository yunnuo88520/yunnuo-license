package crypto

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/hex"
	"strings"
)

func NewID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])
	return prefix + "_" + strings.ToLower(encoded)
}

// ApplicationID follows an 18-character AppID layout with this platform's prefix.
func ApplicationID() (string, error) {
	value, err := randomHex(8)
	return "yn" + value, err
}

func ApplicationSecret() (string, error) { return randomHex(16) }

func randomHex(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
