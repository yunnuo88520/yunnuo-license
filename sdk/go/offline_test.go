package ynlicense

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"
	"time"
)

func TestVerifyOfflineCacheToken(t *testing.T) {
	privateKey, publicPEM := testKeyPair(t)
	now := time.Date(2026, 8, 10, 3, 0, 0, 0, time.UTC)
	licenseExpiry := now.Add(48 * time.Hour)
	claims := OfflineCacheClaims{
		Type:             "offline_cache",
		LicenseNo:        "lic_test",
		ProductCode:      "YN",
		BindMode:         "device",
		BindDigest:       BindingDigest("device", "machine-A"),
		LicenseExpiredAt: &licenseExpiry,
		OfflineUntil:     now.Add(24 * time.Hour),
		IssuedAt:         now,
		KeyVersion:       1,
	}
	token := testSignJSON(t, privateKey, claims)
	verifier, err := NewOfflineVerifier(publicPEM, "YN", WithVerifierClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	verified, err := verifier.VerifyOfflineToken(token, OfflineExpectation{
		LicenseNo: "lic_test",
		BindMode:  "device",
		BindValue: "machine-A",
	})
	if err != nil || verified.LicenseNo != claims.LicenseNo {
		t.Fatalf("verify token: claims=%#v err=%v", verified, err)
	}
	_, err = verifier.VerifyOfflineToken(token, OfflineExpectation{LicenseNo: "lic_test", BindMode: "device", BindValue: "machine-B"})
	if !IsVerificationErrorCode(err, VerificationBindingMismatch) {
		t.Fatalf("expected binding mismatch, got %v", err)
	}

	separator := strings.IndexByte(token, '.') + 1
	tampered := token[:separator] + differentBase64Char(token[separator]) + token[separator+1:]
	_, err = verifier.VerifyOfflineToken(tampered, OfflineExpectation{LicenseNo: "lic_test", BindMode: "device", BindValue: "machine-A"})
	if !IsVerificationErrorCode(err, VerificationInvalidSignature) {
		t.Fatalf("expected invalid signature, got %v", err)
	}

	claims.OfflineUntil = now.Add(-time.Second)
	expiredToken := testSignJSON(t, privateKey, claims)
	_, err = verifier.VerifyOfflineToken(expiredToken, OfflineExpectation{LicenseNo: "lic_test", BindMode: "device", BindValue: "machine-A"})
	if !IsVerificationErrorCode(err, VerificationOfflineWindowExpired) {
		t.Fatalf("expected offline window expiry, got %v", err)
	}
}

func TestPerpetualOfflineUsageSurvivesUpdateExpiry(t *testing.T) {
	key, publicPEM := testKeyPair(t)
	now := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Hour)
	claims := OfflineCacheClaims{Type: "offline_cache", LicenseNo: "lic_perpetual", ProductCode: "YN", LicenseMode: "perpetual", BindMode: "device", BindDigest: BindingDigest("device", "machine-A"), UpdatesExpiredAt: &expired, OfflineUntil: now.Add(time.Hour), IssuedAt: now, KeyVersion: 1}
	verifier, err := NewOfflineVerifier(publicPEM, "YN", WithVerifierClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	result, err := verifier.VerifyOfflineToken(testSignJSON(t, key, claims), OfflineExpectation{BindMode: "device", BindValue: "machine-A"})
	if err != nil || result.UsageExpiredAt != nil || !result.UpdatesExpiredAt.Equal(expired) {
		t.Fatalf("maintenance expiry invalidated offline usage: %+v %v", result, err)
	}
}

func TestVerifyOfflineLicenseFile(t *testing.T) {
	privateKey, publicPEM := testKeyPair(t)
	now := time.Date(2026, 8, 10, 3, 0, 0, 0, time.UTC)
	expiresAt := now.Add(365 * 24 * time.Hour)
	claims := OfflineLicenseClaims{
		Version:     1,
		LicenseNo:   "off_test",
		ProductID:   "prod_test",
		ProductCode: "YN",
		ProductName: "Test Product",
		AppKey:      "app_test",
		BindMode:    "device",
		MachineCode: "MACHINE-ABC",
		IssuedAt:    now,
		ExpiredAt:   &expiresAt,
	}
	content, err := json.Marshal(OfflineLicenseFile{
		Format:  "yn-license-key",
		Version: 1,
		Token:   testSignJSON(t, privateKey, claims),
	})
	if err != nil {
		t.Fatalf("marshal file: %v", err)
	}
	verifier, err := NewOfflineVerifier(publicPEM, "YN", WithVerifierClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	verified, err := verifier.VerifyLicenseFile(content, "machine-abc")
	if err != nil || verified.LicenseNo != "off_test" {
		t.Fatalf("verify license file: claims=%#v err=%v", verified, err)
	}
	_, err = verifier.VerifyLicenseFile(content, "machine-other")
	if !IsVerificationErrorCode(err, VerificationBindingMismatch) {
		t.Fatalf("expected machine mismatch, got %v", err)
	}

	claims.IssuedAt = now.Add(10 * time.Minute)
	futureContent, _ := json.Marshal(OfflineLicenseFile{Format: "yn-license-key", Version: 1, Token: testSignJSON(t, privateKey, claims)})
	_, err = verifier.VerifyLicenseFile(futureContent, "machine-abc")
	if !IsVerificationErrorCode(err, VerificationIssuedInFuture) {
		t.Fatalf("expected future issue time, got %v", err)
	}
}

func testKeyPair(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
	return key, string(publicPEM)
}

func testSignJSON(t *testing.T, key *rsa.PrivateKey, payload any) string {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	sum := sha256.Sum256(body)
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatalf("sign payload: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func differentBase64Char(value byte) string {
	if value == 'A' {
		return "B"
	}
	return "A"
}

func TestKeyRingVerifierSelectsKeyByVersion(t *testing.T) {
	privateV1, publicV1 := testKeyPair(t)
	privateV2, publicV2 := testKeyPair(t)
	now := time.Date(2026, 8, 10, 3, 0, 0, 0, time.UTC)
	licenseExpiry := now.Add(48 * time.Hour)
	base := OfflineCacheClaims{
		Type:             "offline_cache",
		LicenseNo:        "lic_ring",
		ProductCode:      "YN",
		BindMode:         "device",
		BindDigest:       BindingDigest("device", "machine-A"),
		LicenseExpiredAt: &licenseExpiry,
		OfflineUntil:     now.Add(24 * time.Hour),
		IssuedAt:         now,
		KeyVersion:       1,
	}

	verifier, err := NewKeyRingOfflineVerifier(map[int]string{1: publicV1, 2: publicV2}, "YN",
		WithVerifierClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("new key ring verifier: %v", err)
	}

	// v2 签发的新 token（密钥轮换后）与 v1 旧 token 都能通过。
	v2Token := testSignJSON(t, privateV2, func() OfflineCacheClaims { c := base; c.KeyVersion = 2; return c }())
	if _, err := verifier.VerifyOfflineToken(v2Token, OfflineExpectation{BindMode: "device", BindValue: "machine-A"}); err != nil {
		t.Fatalf("v2 token should verify: %v", err)
	}
	v1Token := testSignJSON(t, privateV1, base)
	if _, err := verifier.VerifyOfflineToken(v1Token, OfflineExpectation{BindMode: "device", BindValue: "machine-A"}); err != nil {
		t.Fatalf("v1 token should verify: %v", err)
	}

	// 未知版本拒绝。
	v9Token := testSignJSON(t, privateV2, func() OfflineCacheClaims { c := base; c.KeyVersion = 9; return c }())
	err = func() error {
		_, err := verifier.VerifyOfflineToken(v9Token, OfflineExpectation{BindMode: "device", BindValue: "machine-A"})
		return err
	}()
	if !IsVerificationErrorCode(err, VerificationKeyVersionUnknown) {
		t.Fatalf("unknown version should fail with KEY_VERSION_UNKNOWN: %v", err)
	}

	// 环外私钥伪造 key_version=2 不能通过（签名校验仍然生效）。
	foreignPrivate, _ := testKeyPair(t)
	foreignToken := testSignJSON(t, foreignPrivate, func() OfflineCacheClaims { c := base; c.KeyVersion = 2; return c }())
	err = func() error {
		_, err := verifier.VerifyOfflineToken(foreignToken, OfflineExpectation{BindMode: "device", BindValue: "machine-A"})
		return err
	}()
	if !IsVerificationErrorCode(err, VerificationInvalidSignature) {
		t.Fatalf("forged token should fail signature: %v", err)
	}
}
