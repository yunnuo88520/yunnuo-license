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
	"fmt"
	"net/netip"
	"net/url"
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
	VerificationKeyVersionUnknown    = "KEY_VERSION_UNKNOWN"
)

type OfflineVerifier struct {
	publicKey   *rsa.PublicKey
	keyRing     map[int]*rsa.PublicKey
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

// NewKeyRingOfflineVerifier 构造按 token 的 key_version 选钥的离线验签器：
// 密钥轮换后，携带着各版本公钥的已部署客户端仍能验证新旧 token。
// keys 的键为密钥版本号，值为其 SPKI PEM 公钥。
func NewKeyRingOfflineVerifier(keys map[int]string, productCode string, options ...VerifierOption) (*OfflineVerifier, error) {
	if len(keys) == 0 {
		return nil, errors.New("ynlicense: key ring must contain at least one key")
	}
	ring := make(map[int]*rsa.PublicKey, len(keys))
	for version, pem := range keys {
		if version < 1 {
			return nil, fmt.Errorf("ynlicense: invalid key version %d", version)
		}
		key, err := parsePublicKey(pem)
		if err != nil {
			return nil, fmt.Errorf("ynlicense: invalid public key for version %d: %w", version, err)
		}
		ring[version] = key
	}
	productCode = strings.TrimSpace(productCode)
	if productCode == "" {
		return nil, errors.New("ynlicense: product code is required")
	}
	verifier := &OfflineVerifier{
		keyRing:     ring,
		productCode: productCode,
		clock:       time.Now,
		clockSkew:   5 * time.Minute,
	}
	for _, option := range options {
		option(verifier)
	}
	return verifier, nil
}

// selectKey 先解析未验证的 claims 读取 key_version 选出验签公钥（密钥环模式），
// 验签通过之前 claims 不可信任；单公钥模式直接返回固定公钥。
func (v *OfflineVerifier) selectKey(token string) (*rsa.PublicKey, OfflineCacheClaims, error) {
	if v.publicKey != nil {
		return v.publicKey, OfflineCacheClaims{}, nil
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, OfflineCacheClaims{}, verificationError(VerificationInvalidSignature, "invalid signed token")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, OfflineCacheClaims{}, verificationError(VerificationInvalidSignature, err.Error())
	}
	var probe OfflineCacheClaims
	if err := json.Unmarshal(body, &probe); err != nil {
		return nil, OfflineCacheClaims{}, verificationError(VerificationInvalidFile, "signed claims are not valid JSON")
	}
	key := v.keyRing[probe.KeyVersion]
	if key == nil {
		return nil, OfflineCacheClaims{}, verificationError(VerificationKeyVersionUnknown,
			fmt.Sprintf("no public key for key_version %d", probe.KeyVersion))
	}
	return key, OfflineCacheClaims{}, nil
}

type OfflineExpectation struct {
	SessionID  string
	InstanceID string
	SeatID     string
	LicenseNo  string
	BindMode   string
	BindValue  string
}

type OfflineCacheClaims struct {
	InstanceID         string         `json:"instance_id"`
	SeatID             string         `json:"seat_id"`
	SessionID          string         `json:"session_id"`
	Purpose            string         `json:"purpose"`
	CanUse             bool           `json:"can_use"`
	AuthorizationTerms map[string]any `json:"authorization_terms"`
	LicenseMode        string         `json:"license_mode"`
	UsageExpiredAt     *time.Time     `json:"usage_expired_at"`
	UpdatesExpiredAt   *time.Time     `json:"updates_expired_at"`
	Type               string         `json:"type"`
	LicenseNo          string         `json:"license_no"`
	ProductCode        string         `json:"product_code"`
	BindMode           string         `json:"bind_mode"`
	BindHash           string         `json:"bind_hash"`
	BindDigest         string         `json:"bind_digest"`
	LicenseExpiredAt   *time.Time     `json:"license_expired_at,omitempty"`
	OfflineUntil       time.Time      `json:"offline_until"`
	IssuedAt           time.Time      `json:"issued_at"`
	KeyVersion         int            `json:"key_version"`
}

func (v *OfflineVerifier) VerifyOfflineToken(token string, expected OfflineExpectation) (OfflineCacheClaims, error) {
	key, _, err := v.selectKey(token)
	if err != nil {
		return OfflineCacheClaims{}, err
	}
	claims, err := verifySignedJSON[OfflineCacheClaims](key, token)
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
	if expected.InstanceID != "" && claims.InstanceID != expected.InstanceID || expected.SeatID != "" && claims.SeatID != expected.SeatID || expected.SessionID != "" && claims.SessionID != expected.SessionID {
		return OfflineCacheClaims{}, verificationError(VerificationBindingMismatch, "installation or seat does not match")
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
	key, _, err := v.selectKey(file.Token)
	if err != nil {
		return OfflineLicenseClaims{}, err
	}
	claims, err := verifySignedJSON[OfflineLicenseClaims](key, file.Token)
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
	value := strings.TrimSpace(bindValue)
	mode := strings.ToLower(strings.TrimSpace(bindMode))
	switch mode {
	case "tenant", "instance", "cluster":
	case "ip", "ip_cidr":
		value = canonicalDigestComponent(mode, value)
	case "domain", "domain_wildcard":
		value = canonicalDigestComponent(mode, value)
	case "composite":
		var fields map[string]string
		if json.Unmarshal([]byte(value), &fields) == nil {
			for kind, v := range fields {
				fields[kind] = canonicalDigestComponent(kind, v)
			}
			if raw, e := json.Marshal(fields); e == nil {
				value = string(raw)
			}
		}
	default:
		value = strings.ToLower(value)
	}
	sum.Write([]byte(value))
	return hex.EncodeToString(sum.Sum(nil))
}

func canonicalDigestComponent(mode, value string) string {
	value = strings.TrimSpace(value)
	switch mode {
	case "ip", "ip_cidr":
		if ip, e := netip.ParseAddr(value); e == nil {
			return ip.Unmap().String()
		}
		if prefix, e := netip.ParsePrefix(value); e == nil {
			return prefix.Masked().String()
		}
	case "domain", "domain_wildcard":
		wild := strings.HasPrefix(value, "*.")
		value = strings.TrimPrefix(value, "*.")
		if !strings.Contains(value, "://") {
			value = "http://" + value
		}
		if parsed, e := url.Parse(value); e == nil {
			value = strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
			if wild {
				value = "*." + value
			}
			return value
		}
	case "account":
		return strings.ToLower(value)
	}
	return value
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
