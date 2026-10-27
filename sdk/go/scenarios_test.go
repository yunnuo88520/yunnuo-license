package ynlicense

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestPersistentInstallationAndActualRequestHost(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	ids := make(chan string, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := OpenInstallation(dir, "hardware-provider-id")
			if e != nil {
				t.Error(e)
				return
			}
			ids <- v.InstanceID
		}()
	}
	wg.Wait()
	close(ids)
	id := ""
	for got := range ids {
		if id != "" && got != id {
			t.Fatal("concurrent startup replaced the installation")
		}
		id = got
	}
	info, e := os.Stat(filepath.Join(dir, "instance.json"))
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatal("identity must be private", e)
	}
	if _, e = OpenInstallation(dir, "other-hardware"); e == nil {
		t.Fatal("changed device accepted")
	}
	in := VerifyRequest{BindMode: "domain_wildcard", BindValue: "*.example.com"}
	req := &http.Request{Host: "wrong.example.net:443", RemoteAddr: "10.0.0.2:4567", Header: http.Header{"X-Forwarded-Host": []string{"app.example.com"}}}
	actual, e := BindingForHTTPRequest(in, req, nil)
	if e != nil || actual.BindValue != "wrong.example.net" {
		t.Fatal(actual, e)
	}
	actual, e = BindingForHTTPRequest(in, req, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/24")})
	if e != nil || actual.BindValue != "app.example.com" {
		t.Fatal(actual, e)
	}
}
func TestSignedReleaseTargetsAndSourcePath(t *testing.T) {
	key, pem := testKeyPair(t)
	content := []byte("program-v2")
	hash := sha256.Sum256(content)
	target := &ReleaseTarget{OS: "linux", Arch: "amd64", Runtime: "go", Channel: "stable", Kind: "archive", MinVersion: "1.0.0", MaxVersion: "1.9.9"}
	claims := releaseClaims{Type: "product_release", ReleaseID: "release-A", ProductID: "app-A", Version: "2.0.0", SHA256: hex.EncodeToString(hash[:]), Size: int64(len(content)), KeyVersion: 1, Target: target}
	release := ProductRelease{ID: claims.ReleaseID, ProductID: claims.ProductID, Version: claims.Version, SHA256: claims.SHA256, Size: claims.Size, KeyVersion: 1, Target: target, ManifestToken: testSignJSON(t, key, claims)}
	expected := ReleaseExpectation{PublicKeyPEM: pem, ProductID: "app-A", Target: target, CurrentVersion: "1.0.0"}
	if e := VerifyReleasePackage(release, content, expected); e != nil {
		t.Fatal(e)
	}
	wrong := *target
	wrong.OS = "windows"
	expected.Target = &wrong
	if VerifyReleasePackage(release, content, expected) == nil {
		t.Fatal("valid signed package for wrong OS accepted")
	}
	expected.Target = target
	expected.CurrentVersion = "0.9.0"
	if VerifyReleaseManifest(release, expected) == nil {
		t.Fatal("unsupported source path accepted")
	}
	tampered := *target
	tampered.Channel = "beta"
	release.Target = &tampered
	expected.CurrentVersion = "1.0.0"
	if VerifyReleaseManifest(release, expected) == nil {
		t.Fatal("unsigned target modification accepted")
	}
}
