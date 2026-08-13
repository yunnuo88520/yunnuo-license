package crypto

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

const cardAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func NormalizeCardCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	code = strings.ReplaceAll(code, "-", "")
	code = strings.ReplaceAll(code, " ", "")
	return code
}

func FormatCardCode(productPrefix, normalized string) string {
	parts := []string{strings.ToUpper(productPrefix)}
	for i := 0; i < len(normalized); i += 4 {
		end := i + 4
		if end > len(normalized) {
			end = len(normalized)
		}
		parts = append(parts, normalized[i:end])
	}
	return strings.Join(parts, "-")
}

func GenerateCardCode(productPrefix string) (string, error) {
	body, err := randomAlphabet(12)
	if err != nil {
		return "", err
	}
	check := checksum(productPrefix + body)
	return FormatCardCode(productPrefix, body+check), nil
}

func GenerateAgentLoginCode() (string, error) {
	body, err := randomAlphabet(6)
	if err != nil {
		return "", err
	}
	return "YN-" + body, nil
}

func ValidateChecksum(productPrefix, code string) bool {
	normalized := NormalizeCardCode(code)
	prefix := strings.ToUpper(productPrefix)
	normalized = strings.TrimPrefix(normalized, prefix)
	if len(normalized) < 3 {
		return false
	}
	body := normalized[:len(normalized)-2]
	check := normalized[len(normalized)-2:]
	return checksum(prefix+body) == check
}

func CardHash(pepper []byte, code string) string {
	mac := hmac.New(sha256.New, pepper)
	mac.Write([]byte(NormalizeCardCode(code)))
	return hex.EncodeToString(mac.Sum(nil))
}

func BindHash(pepper []byte, bindMode, bindValue string) string {
	mac := hmac.New(sha256.New, pepper)
	mac.Write([]byte(strings.ToLower(strings.TrimSpace(bindMode))))
	mac.Write([]byte{0})
	mac.Write([]byte(strings.ToLower(strings.TrimSpace(bindValue))))
	return hex.EncodeToString(mac.Sum(nil))
}

func BindDigest(bindMode, bindValue string) string {
	sum := sha256.New()
	sum.Write([]byte(strings.ToLower(strings.TrimSpace(bindMode))))
	sum.Write([]byte{0})
	sum.Write([]byte(strings.ToLower(strings.TrimSpace(bindValue))))
	return hex.EncodeToString(sum.Sum(nil))
}

func randomAlphabet(n int) (string, error) {
	var out strings.Builder
	out.Grow(n)
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for _, b := range buf {
		out.WriteByte(cardAlphabet[int(b)%len(cardAlphabet)])
	}
	return out.String(), nil
}

func checksum(input string) string {
	sum := sha256.Sum256([]byte(strings.ToUpper(input)))
	a := cardAlphabet[int(sum[0])%len(cardAlphabet)]
	b := cardAlphabet[int(sum[1])%len(cardAlphabet)]
	return string([]byte{a, b})
}

func EncodeBinary(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func DecodeBinary(value string) ([]byte, error) {
	if value == "" {
		return nil, errors.New("empty value")
	}
	return base64.RawURLEncoding.DecodeString(value)
}
