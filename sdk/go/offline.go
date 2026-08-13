package ynlicense

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"strings"
	"time"
)

const (
	VerificationInvalidFile          = "INVALID_FILE"
	VerificationInvalidSignature     = "INVALID_SIGNATURE"
	VerificationWrongProduct         = "WRONG_PRODUCT"
	VerificationWrongLicense         = "WRONG_LICENSE"
	VerificationBindingMismatch      = "BINDING_MISMATCH"
	VerificationLicenseExpired       = "LICENSE_EXPIRED"
	VerificationOfflineWindowExpired = "OFFLINE_WINDOW_EXPIRED"
	VerificationIssuedInFuture       = "ISSUED_IN_FUTURE"
	VerificationClockRollback        = "CLOCK_ROLLBACK"
)

type OfflineVerifier struct {
	publicKey   *rsa.PublicKey
	productCode string
	clock       func() time.Time
	clockSkew   time.Duration
}

type VerifierOption func(*OfflineVerifier)

func WithVerifierClock(clock func() time.Time) VerifierOption {
	return func(verifier *OfflineVerifier) {
		if clock != nil {
			verifier.clock = clock
		}
	}
}

func WithVerifierClockSkew(skew time.Duration) VerifierOption {
	return func(verifier *OfflineVerifier) {
		if skew >= 0 {
			verifier.clockSkew = skew
		}
	}
}

func NewOfflineVerifier(publicKeyPEM, productCode string, options ...VerifierOption) (*OfflineVerifier, error) {
	key, err := parsePublicKey(publicKeyPEM)
	if err != nil {
		return nil, err
	}
	productCode = strings.TrimSpace(productCode)
	if productCode == "" {
		return nil, errors.New("ynlicense: product code is required")
	}
	verifier := &OfflineVerifier{
		publicKey:   key,
		productCode: productCode,
		clock:       time.Now,
		clockSkew:   5 * time.Minute,
	}
	for _, option := range options {
		option(verifier)
	}
	return verifier, nil
}

type OfflineExpectation struct {
	LicenseNo string
	BindMode  string
	BindValue string
}

type OfflineCacheClaims struct {
	Type             string     `json:"type"`
	LicenseNo        string     `json:"license_no"`
	ProductCode      string     `json:"product_code"`
	BindMode         string     `json:"bind_mode"`
	BindHash         string     `json:"bind_hash"`
	BindDigest       string     `json:"bind_digest"`
	LicenseExpiredAt *time.Time `json:"license_expired_at,omitempty"`
	OfflineUntil     time.Time  `json:"offline_until"`
	IssuedAt         time.Time  `json:"issued_at"`
	KeyVersion       int        `json:"key_version"`
}

func (v *OfflineVerifier) VerifyOfflineToken(token string, expected OfflineExpectation) (OfflineCacheClaims, error) {
	claims, err := verifySignedJSON[OfflineCacheClaims](v.publicKey, token)
	if err != nil {
		return OfflineCacheClaims{}, verificationError(VerificationInvalidSignature, err.Error())
	}
	if claims.Type != "offline_cache" || claims.LicenseNo == "" || claims.OfflineUntil.IsZero() {
		return OfflineCacheClaims{}, verificationError(VerificationInvalidFile, "invalid offline cache claims")
	}
	if !equalFoldTrim(claims.ProductCode, v.productCode) {
		return OfflineCacheClaims{}, verificationError(VerificationWrongProduct, "product code does not match")
	}
	if expected.LicenseNo != "" && strings.TrimSpace(claims.LicenseNo) != strings.TrimSpace(expected.LicenseNo) {
		return OfflineCacheClaims{}, verificationError(VerificationWrongLicense, "license number does not match")
	}
	if !equalFoldTrim(claims.BindMode, expected.BindMode) || claims.BindDigest == "" {
		return OfflineCacheClaims{}, verificationError(VerificationBindingMismatch, "binding mode or digest does not match")
	}
	expectedDigest := BindingDigest(expected.BindMode, expected.BindValue)
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(claims.BindDigest)), []byte(expectedDigest)) != 1 {
		return OfflineCacheClaims{}, verificationError(VerificationBindingMismatch, "binding value does not match")
	}
	now := v.clock().UTC()
	if claims.IssuedAt.After(now.Add(v.clockSkew)) {
		return OfflineCacheClaims{}, verificationError(VerificationIssuedInFuture, "token issue time is in the future")
	}
	if claims.LicenseExpiredAt != nil && !now.Before(*claims.LicenseExpiredAt) {
		return OfflineCacheClaims{}, verificationError(VerificationLicenseExpired, "license has expired")
	}
	if !now.Before(claims.OfflineUntil) {
		return OfflineCacheClaims{}, verificationError(VerificationOfflineWindowExpired, "offline window has expired")
	}
	return claims, nil
}

type OfflineLicenseFile struct {
	Format  string `json:"format"`
	Version int    `json:"version"`
	Token   string `json:"token"`
}

type OfflineLicenseClaims struct {
	Version     int        `json:"version"`
	KeyVersion  int        `json:"key_version"`
	LicenseNo   string     `json:"license_no"`
	ProductID   string     `json:"product_id"`
	ProductCode string     `json:"product_code"`
	ProductName string     `json:"product_name"`
	AppKey      string     `json:"app_key"`
	BindMode    string     `json:"bind_mode"`
	MachineCode string     `json:"machine_code"`
	IssuedAt    time.Time  `json:"issued_at"`
	ExpiredAt   *time.Time `json:"expired_at,omitempty"`
	IsPermanent bool       `json:"is_permanent"`
}

func (v *OfflineVerifier) VerifyLicenseFile(content []byte, machineCode string) (OfflineLicenseClaims, error) {
	var file OfflineLicenseFile
	if err := json.Unmarshal(content, &file); err != nil {
		return OfflineLicenseClaims{}, verificationError(VerificationInvalidFile, "file is not valid JSON")
	}
	if file.Format != "yn-license-key" || file.Version != 1 || file.Token == "" {
		return OfflineLicenseClaims{}, verificationError(VerificationInvalidFile, "unsupported file format or version")
	}
	claims, err := verifySignedJSON[OfflineLicenseClaims](v.publicKey, file.Token)
	if err != nil {
		return OfflineLicenseClaims{}, verificationError(VerificationInvalidSignature, err.Error())
	}
	if claims.Version != file.Version || claims.LicenseNo == "" || claims.BindMode != "device" {
		return OfflineLicenseClaims{}, verificationError(VerificationInvalidFile, "invalid offline license claims")
	}
	if !equalFoldTrim(claims.ProductCode, v.productCode) {
		return OfflineLicenseClaims{}, verificationError(VerificationWrongProduct, "product code does not match")
	}
	if !equalFoldTrim(claims.MachineCode, machineCode) {
		return OfflineLicenseClaims{}, verificationError(VerificationBindingMismatch, "machine code does not match")
	}
	now := v.clock().UTC()
	if claims.IssuedAt.After(now.Add(v.clockSkew)) {
		return OfflineLicenseClaims{}, verificationError(VerificationIssuedInFuture, "license issue time is in the future")
	}
	if !claims.IsPermanent {
		if claims.ExpiredAt == nil || !now.Before(*claims.ExpiredAt) {
			return OfflineLicenseClaims{}, verificationError(VerificationLicenseExpired, "license has expired")
		}
	}
	return claims, nil
}

func BindingDigest(bindMode, bindValue string) string {
	sum := sha256.New()
	sum.Write([]byte(strings.ToLower(strings.TrimSpace(bindMode))))
	sum.Write([]byte{0})
	sum.Write([]byte(strings.ToLower(strings.TrimSpace(bindValue))))
	return hex.EncodeToString(sum.Sum(nil))
}

func verifySignedJSON[T any](key *rsa.PublicKey, token string) (T, error) {
	var target T
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return target, errors.New("invalid signed token")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return target, err
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return target, err
	}
	sum := sha256.Sum256(body)
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], signature); err != nil {
		return target, err
	}
	if err := json.Unmarshal(body, &target); err != nil {
		return target, err
	}
	return target, nil
}

func parsePublicKey(value string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(value))
	if block == nil {
		return nil, errors.New("ynlicense: invalid public key PEM")
	}
	if parsed, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		if key, ok := parsed.(*rsa.PublicKey); ok {
			return key, nil
		}
	}
	if key, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, errors.New("ynlicense: PEM does not contain an RSA public key")
}

func equalFoldTrim(left, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}
