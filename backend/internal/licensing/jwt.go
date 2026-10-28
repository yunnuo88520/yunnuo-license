package licensing

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const TokenIssuer = "urn:yunnuo:licensing:v2"

type BindingClaim struct {
	Type string `json:"type"`
	Hash string `json:"hash"`
}
type JWTClaims struct {
	Release      *ReleaseManifest `json:"release,omitempty"`
	Issuer       string           `json:"iss"`
	Audience     string           `json:"aud"`
	Subject      string           `json:"sub"`
	JWTID        string           `json:"jti"`
	IssuedAt     int64            `json:"iat"`
	NotBefore    int64            `json:"nbf"`
	ExpiresAt    int64            `json:"exp"`
	TokenUse     string           `json:"token_use"`
	ActivationID string           `json:"activation_id,omitempty"`
	Binding      BindingClaim     `json:"binding"`
	Model        string           `json:"model,omitempty"`
	Perpetual    bool             `json:"perpetual"`
	Features     []string         `json:"features,omitempty"`
	RefreshAfter int64            `json:"refresh_after,omitempty"`
	GraceUntil   int64            `json:"grace_until,omitempty"`
	UpdatesUntil *int64           `json:"updates_until"`
}
type JWTHeader struct {
	Algorithm string `json:"alg"`
	Type      string `json:"typ"`
	KID       string `json:"kid"`
}
type TokenExpectation struct {
	ApplicationID, Subject, ActivationID, TokenUse, BindingType, BindingHash string
	Now                                                                      time.Time
}
type TokenError struct{ Code string }

func (e *TokenError) Error() string { return "licensing: " + e.Code }
func tokenError(code string) error  { return &TokenError{code} }

var jwtSegment = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func SignJWT(privatePEM, kid string, claims JWTClaims) (string, error) {
	if kid == "" || claims.Issuer != TokenIssuer || claims.Audience == "" || claims.Subject == "" || claims.JWTID == "" || claims.TokenUse == "" || claims.ExpiresAt <= claims.IssuedAt {
		return "", errors.New("licensing: incomplete JWT claims")
	}
	block, _ := pem.Decode([]byte(privatePEM))
	if block == nil {
		return "", errors.New("licensing: invalid private key")
	}
	key, e := x509.ParsePKCS1PrivateKey(block.Bytes)
	if e != nil {
		return "", e
	}
	if key.N.BitLen() < 2048 {
		return "", errors.New("licensing: RSA key is too small")
	}
	header, e := json.Marshal(JWTHeader{"RS256", "JWT", kid})
	if e != nil {
		return "", e
	}
	payload, e := json.Marshal(claims)
	if e != nil {
		return "", e
	}
	message := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(message))
	signature, e := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if e != nil {
		return "", e
	}
	return message + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}
func DecodeJWTHeader(token string) (JWTHeader, error) {
	var header JWTHeader
	parts := strings.Split(token, ".")
	if len(token) > 16384 || len(parts) != 3 {
		return header, tokenError("TOKEN_MALFORMED")
	}
	for _, part := range parts {
		if !jwtSegment.MatchString(part) {
			return header, tokenError("TOKEN_MALFORMED")
		}
	}
	raw, e := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if e != nil || len(raw) > 2048 {
		return header, tokenError("TOKEN_MALFORMED")
	}
	if json.Unmarshal(raw, &header) != nil || header.Algorithm != "RS256" || header.Type != "JWT" || header.KID == "" || len(header.KID) > 160 {
		return header, tokenError("TOKEN_ALGORITHM_INVALID")
	}
	return header, nil
}
func VerifyJWT(token, publicPEM string, expected TokenExpectation) (JWTClaims, error) {
	return verifyJWT(token, publicPEM, expected, false)
}

// A refresh request authenticates the installation key independently and checks
// current server state. An expired access token is only an association hint.
func verifyJWTForRefresh(token, publicPEM string, expected TokenExpectation) (JWTClaims, error) {
	return verifyJWT(token, publicPEM, expected, true)
}
func verifyJWT(token, publicPEM string, expected TokenExpectation, associationOnly bool) (JWTClaims, error) {
	var claims JWTClaims
	if _, e := DecodeJWTHeader(token); e != nil {
		return claims, e
	}
	parts := strings.Split(token, ".")
	block, _ := pem.Decode([]byte(publicPEM))
	if block == nil {
		return claims, tokenError("TOKEN_KEY_INVALID")
	}
	parsed, e := x509.ParsePKIXPublicKey(block.Bytes)
	if e != nil {
		return claims, tokenError("TOKEN_KEY_INVALID")
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok || key.N.BitLen() < 2048 {
		return claims, tokenError("TOKEN_KEY_INVALID")
	}
	signature, e := base64.RawURLEncoding.Strict().DecodeString(parts[2])
	if e != nil {
		return claims, tokenError("TOKEN_SIGNATURE_INVALID")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature) != nil {
		return claims, tokenError("TOKEN_SIGNATURE_INVALID")
	}
	payload, e := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if e != nil || json.Unmarshal(payload, &claims) != nil {
		return claims, tokenError("TOKEN_MALFORMED")
	}
	if expected.ApplicationID == "" || claims.Issuer != TokenIssuer || claims.Audience != expected.ApplicationID || claims.Subject == "" || claims.JWTID == "" || claims.ExpiresAt == 0 || claims.IssuedAt == 0 || claims.TokenUse != expected.TokenUse {
		return claims, tokenError("TOKEN_SCOPE_INVALID")
	}
	if expected.Subject != "" && claims.Subject != expected.Subject || expected.ActivationID != "" && claims.ActivationID != expected.ActivationID || expected.BindingType != "" && claims.Binding.Type != expected.BindingType || expected.BindingHash != "" && claims.Binding.Hash != expected.BindingHash {
		return claims, tokenError("TOKEN_BINDING_MISMATCH")
	}
	now := expected.Now
	if now.IsZero() {
		now = time.Now()
	}
	if claims.IssuedAt > now.Add(2*time.Minute).Unix() || claims.NotBefore > now.Add(30*time.Second).Unix() {
		return claims, tokenError("TOKEN_NOT_YET_VALID")
	}
	if claims.ExpiresAt <= claims.IssuedAt {
		return claims, tokenError("TOKEN_MALFORMED")
	}
	if !associationOnly && now.Unix() >= claims.ExpiresAt {
		return claims, tokenError("TOKEN_EXPIRED")
	}
	return claims, nil
}

type JWK struct {
	KTY       string `json:"kty"`
	Use       string `json:"use"`
	Algorithm string `json:"alg"`
	KID       string `json:"kid"`
	N         string `json:"n"`
	E         string `json:"e"`
}
type JWKS struct {
	Keys []JWK `json:"keys"`
}

func PublicJWK(publicPEM, kid string) (JWK, error) {
	block, _ := pem.Decode([]byte(publicPEM))
	if block == nil {
		return JWK{}, errors.New("licensing: invalid public key")
	}
	parsed, e := x509.ParsePKIXPublicKey(block.Bytes)
	if e != nil {
		return JWK{}, e
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok || key.N.BitLen() < 2048 {
		return JWK{}, errors.New("licensing: invalid RSA public key")
	}
	return JWK{KTY: "RSA", Use: "sig", Algorithm: "RS256", KID: kid, N: base64.RawURLEncoding.EncodeToString(key.N.Bytes()), E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}, nil
}
func RequestSigningMessage(method, path, appID string, timestamp int64, nonce string, body []byte) string {
	sum := sha256.Sum256(body)
	return strings.Join([]string{"YN-AUTH-V2", strings.ToUpper(method), path, appID, strconv.FormatInt(timestamp, 10), nonce, hex.EncodeToString(sum[:])}, "\n")
}

type RequestProof struct {
	Timestamp int64  `json:"timestamp"`
	Nonce     string `json:"nonce"`
	Signature string `json:"signature"`
}

func VerifyInstallationRequest(publicKey, method, path, appID string, body []byte, proof RequestProof, now time.Time) error {
	if proof.Timestamp <= 0 || now.Sub(time.Unix(proof.Timestamp, 0)) > 2*time.Minute || time.Unix(proof.Timestamp, 0).Sub(now) > 2*time.Minute {
		return tokenError("REQUEST_TIMESTAMP_INVALID")
	}
	if len(proof.Nonce) < 22 || len(proof.Nonce) > 86 || !jwtSegment.MatchString(proof.Nonce) {
		return tokenError("REQUEST_NONCE_INVALID")
	}
	key, e := base64.RawURLEncoding.Strict().DecodeString(publicKey)
	if e != nil || len(key) != ed25519.PublicKeySize {
		return tokenError("INSTALLATION_KEY_INVALID")
	}
	signature, e := base64.RawURLEncoding.Strict().DecodeString(proof.Signature)
	if e != nil || len(signature) != ed25519.SignatureSize {
		return tokenError("REQUEST_SIGNATURE_INVALID")
	}
	message := RequestSigningMessage(method, path, appID, proof.Timestamp, proof.Nonce, body)
	if !ed25519.Verify(ed25519.PublicKey(key), []byte(message), signature) {
		return tokenError("REQUEST_SIGNATURE_INVALID")
	}
	return nil
}
func PublicKeyFingerprint(publicKey string) (string, error) {
	raw, e := base64.RawURLEncoding.Strict().DecodeString(publicKey)
	if e != nil || len(raw) != ed25519.PublicKeySize {
		return "", fmt.Errorf("licensing: invalid Ed25519 public key")
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:]), nil
}
