package licensing

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"strings"
	"testing"
	"time"
)

func TestStandardJWTApplicationBindingPurposeAndDeadline(t *testing.T) {
	public, private, e := yncrypto.GenerateRSAKeyPair()
	if e != nil {
		t.Fatal(e)
	}
	other, _, e := yncrypto.GenerateRSAKeyPair()
	if e != nil {
		t.Fatal(e)
	}
	now := time.Unix(1800000000, 0)
	claims := JWTClaims{Issuer: TokenIssuer, Audience: "yn0000000000000001", Subject: "license-A", JWTID: "token-A", IssuedAt: now.Unix(), NotBefore: now.Unix(), ExpiresAt: now.Add(time.Hour).Unix(), TokenUse: "license", ActivationID: "activation-A", Binding: BindingClaim{Type: "machine_id", Hash: "fingerprint-hash"}}
	token, e := SignJWT(private, "yn0000000000000001.v1", claims)
	if e != nil {
		t.Fatal(e)
	}
	if len(strings.Split(token, ".")) != 3 {
		t.Fatal("not standard JWT")
	}
	expected := TokenExpectation{ApplicationID: claims.Audience, Subject: claims.Subject, ActivationID: claims.ActivationID, TokenUse: "license", BindingType: "machine_id", BindingHash: "fingerprint-hash", Now: now}
	if _, e = VerifyJWT(token, public, expected); e != nil {
		t.Fatal(e)
	}
	for name, change := range map[string]func(*TokenExpectation){"application": func(v *TokenExpectation) { v.ApplicationID = "yn0000000000000002" }, "binding": func(v *TokenExpectation) { v.BindingHash = "other" }, "purpose": func(v *TokenExpectation) { v.TokenUse = "offline_grace" }, "activation": func(v *TokenExpectation) { v.ActivationID = "other" }, "exact deadline": func(v *TokenExpectation) { v.Now = time.Unix(claims.ExpiresAt, 0) }} {
		t.Run(name, func(t *testing.T) {
			value := expected
			change(&value)
			if _, e := VerifyJWT(token, public, value); e == nil {
				t.Fatal("invalid token context accepted")
			}
		})
	}
	if _, e = VerifyJWT(token, other, expected); e == nil {
		t.Fatal("another application key accepted")
	}
	parts := strings.Split(token, ".")
	parts[0] = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT","kid":"x"}`))
	if _, e = VerifyJWT(strings.Join(parts, "."), public, expected); e == nil {
		t.Fatal("algorithm substitution accepted")
	}
	jwk, e := PublicJWK(public, "yn0000000000000001.v1")
	if e != nil || jwk.Algorithm != "RS256" || jwk.N == "" || jwk.E != "AQAB" {
		t.Fatal(jwk, e)
	}
	raw, _ := json.Marshal(JWKS{Keys: []JWK{jwk}})
	if strings.Contains(string(raw), "PRIVATE") {
		t.Fatal("JWKS contains private key")
	}
}
func TestInstallationSignatureCoversBodyApplicationPathAndTime(t *testing.T) {
	public, private, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	now := time.Unix(1800000000, 0)
	body := []byte(`{"app_id":"yn0000000000000001"}`)
	nonce := base64.RawURLEncoding.EncodeToString(make([]byte, 24))
	proof := RequestProof{Timestamp: now.Unix(), Nonce: nonce}
	proof.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, []byte(RequestSigningMessage("POST", "/v2/verify", "yn0000000000000001", proof.Timestamp, nonce, body))))
	key := base64.RawURLEncoding.EncodeToString(public)
	if e = VerifyInstallationRequest(key, "POST", "/v2/verify", "yn0000000000000001", body, proof, now); e != nil {
		t.Fatal(e)
	}
	for _, test := range []struct {
		app, path string
		body      []byte
		now       time.Time
	}{{"yn0000000000000002", "/v2/verify", body, now}, {"yn0000000000000001", "/v2/activate", body, now}, {"yn0000000000000001", "/v2/verify", []byte(`{"changed":true}`), now}, {"yn0000000000000001", "/v2/verify", body, now.Add(3 * time.Minute)}} {
		if e = VerifyInstallationRequest(key, "POST", test.path, test.app, test.body, proof, test.now); e == nil {
			t.Fatal("altered request accepted")
		}
	}
	var typed *TokenError
	proof.Signature = "bad"
	if e = VerifyInstallationRequest(key, "POST", "/v2/verify", "yn0000000000000001", body, proof, now); !errors.As(e, &typed) {
		t.Fatal(e)
	}
}
