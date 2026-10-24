package service

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
)

func updateFixture(t *testing.T) (*Service, domain.Product, VerifyInput) {
	t.Helper()
	ctx := context.Background()
	svc := newTestService(t).WithReleaseDir(t.TempDir())
	p := createTestProduct(t, ctx, svc, domain.ConflictReject)
	code := createTestCard(t, ctx, svc, p.ID, 30)
	license, err := svc.Activate(ctx, ActivateInput{AppKey: p.AppKey, CardCode: code, BindMode: domain.BindDevice, BindValue: "ebox-server"})
	if err != nil {
		t.Fatal(err)
	}
	return svc, p, VerifyInput{AppKey: p.AppKey, LicenseNo: license.LicenseNo, BindMode: domain.BindDevice, BindValue: "ebox-server"}
}

func uploadTestRelease(t *testing.T, svc *Service, p domain.Product, version string) domain.ProductRelease {
	t.Helper()
	r, err := svc.UploadRelease(context.Background(), UploadReleaseInput{ProductID: p.ID, Version: version, Notes: "Ebox release", Filename: "ebox.zip"}, strings.NewReader("package contents"), "publisher")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestProductUpdatesOptionalAndPublishedVersionsOrderedNumerically(t *testing.T) {
	ctx := context.Background()
	svc, p, input := updateFixture(t)
	r := uploadTestRelease(t, svc, p, "1.10.0")
	check := CheckUpdateInput{VerifyInput: input, CurrentVersion: "1.9.0"}
	result, err := svc.CheckUpdate(ctx, check)
	if err != nil || result.Enabled || result.UpdateAvailable {
		t.Fatalf("optional default: %+v %v", result, err)
	}
	if _, err := svc.SetProductUpdates(ctx, p.ID, true, "publisher"); err != nil {
		t.Fatal(err)
	}
	result, err = svc.CheckUpdate(ctx, check)
	if err != nil || !result.Enabled || result.UpdateAvailable {
		t.Fatalf("draft visible: %+v %v", result, err)
	}
	for _, version := range []string{"1.10.0", "1.2.0"} {
		release := r
		if version != r.Version {
			release = uploadTestRelease(t, svc, p, version)
		}
		if _, err := svc.ChangeReleaseStatus(ctx, p.ID, release.ID, "published", "publisher"); err != nil {
			t.Fatal(err)
		}
	}
	result, err = svc.CheckUpdate(ctx, check)
	if err != nil || !result.UpdateAvailable || result.Release.ID != r.ID {
		t.Fatalf("latest: %+v %v", result, err)
	}
	for _, version := range []string{"1.10.0", "2.0.0"} {
		check.CurrentVersion = version
		result, err = svc.CheckUpdate(ctx, check)
		if err != nil || result.UpdateAvailable {
			t.Fatalf("unexpected downgrade: %+v %v", result, err)
		}
	}
	_, file, err := svc.DownloadRelease(ctx, r.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(file)
	file.Close()
	if string(body) != "package contents" {
		t.Fatal("wrong download")
	}
	if _, err := svc.ChangeReleaseStatus(ctx, p.ID, r.ID, "withdrawn", "publisher"); err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.DownloadRelease(ctx, r.ID, input)
	assertCode(t, err, "RELEASE_NOT_FOUND")
	if _, err := svc.SetProductUpdates(ctx, p.ID, false, "publisher"); err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.DownloadRelease(ctx, r.ID, input)
	assertCode(t, err, "UPDATES_DISABLED")
}

func TestProductUpdatesRejectExpiredLicenseEvenDuringRuntimeGrace(t *testing.T) {
	ctx := context.Background()
	svc, p, input := updateFixture(t)
	r := uploadTestRelease(t, svc, p, "2.0.0")
	svc.SetProductUpdates(ctx, p.ID, true, "publisher")
	svc.ChangeReleaseStatus(ctx, p.ID, r.ID, "published", "publisher")
	now := svc.now()
	svc.now = func() time.Time { return now.Add(30 * 24 * time.Hour) }
	if _, err := svc.Verify(ctx, input); err != nil {
		t.Fatalf("runtime grace should remain: %v", err)
	}
	_, err := svc.CheckUpdate(ctx, CheckUpdateInput{VerifyInput: input, CurrentVersion: "1.0.0"})
	assertCode(t, err, "LICENSE_EXPIRED")
	_, _, err = svc.DownloadRelease(ctx, r.ID, input)
	assertCode(t, err, "LICENSE_EXPIRED")
}

func TestProductUpdateAuthorizationRejectsWrongBindingRevocationAndProduct(t *testing.T) {
	ctx := context.Background()
	svc, p, input := updateFixture(t)
	r := uploadTestRelease(t, svc, p, "2.0.0")
	svc.SetProductUpdates(ctx, p.ID, true, "publisher")
	svc.ChangeReleaseStatus(ctx, p.ID, r.ID, "published", "publisher")
	bad := input
	bad.BindValue = "another-server"
	_, _, err := svc.DownloadRelease(ctx, r.ID, bad)
	assertCode(t, err, "BINDING_MISMATCH")
	other, err := svc.CreateProduct(ctx, CreateProductInput{Name: "Other", Code: "OTHER", BindMode: domain.BindDevice})
	if err != nil {
		t.Fatal(err)
	}
	otherRelease := uploadTestRelease(t, svc, other, "2.0.0")
	_, _, err = svc.DownloadRelease(ctx, otherRelease.ID, input)
	assertCode(t, err, "RELEASE_NOT_FOUND")
	if _, err = svc.ChangeReleaseStatus(ctx, other.ID, r.ID, "published", "publisher"); err == nil {
		t.Fatal("cross-product status change")
	}
	if _, err = svc.RevokeLicense(ctx, RevokeLicenseInput{LicenseNo: input.LicenseNo, Reason: "test"}); err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.DownloadRelease(ctx, r.ID, input)
	assertCode(t, err, "LICENSE_REVOKED")
}

func TestReleaseManifestIntegrityAndKeyRotation(t *testing.T) {
	ctx := context.Background()
	svc, p, input := updateFixture(t)
	r := uploadTestRelease(t, svc, p, "2.0.0")
	var claims struct {
		Type      string `json:"type"`
		ReleaseID string `json:"release_id"`
		SHA256    string `json:"sha256"`
		ProductID string `json:"product_id"`
	}
	if err := yncrypto.VerifySignedJSON(p.PublicKeyPEM, r.ManifestToken, &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Type != "product_release" || claims.ReleaseID != r.ID || claims.SHA256 != r.SHA256 || claims.ProductID != p.ID {
		t.Fatalf("wrong claims: %+v", claims)
	}
	if _, err := svc.RotateProductKey(ctx, p.ID, "publisher"); err != nil {
		t.Fatal(err)
	}
	newRelease := uploadTestRelease(t, svc, p, "3.0.0")
	if newRelease.KeyVersion != 2 {
		t.Fatal("did not use rotated key")
	}
	if err := yncrypto.VerifySignedJSON(p.PublicKeyPEM, newRelease.ManifestToken, &claims); err == nil {
		t.Fatal("new release accepted by old key")
	}
	svc.SetProductUpdates(ctx, p.ID, true, "publisher")
	svc.ChangeReleaseStatus(ctx, p.ID, r.ID, "published", "publisher")
	if err := os.WriteFile(filepath.Join(svc.releaseDir, r.ID), []byte("tampered package"), 0600); err != nil {
		t.Fatal(err)
	}
	_, _, err := svc.DownloadRelease(ctx, r.ID, input)
	assertCode(t, err, "RELEASE_INTEGRITY_FAILED")
	_, err = svc.ChangeReleaseStatus(ctx, p.ID, r.ID, "published", "publisher")
	assertCode(t, err, "RELEASE_INTEGRITY_FAILED")
}

func TestReleaseUploadValidationAndCleanup(t *testing.T) {
	ctx := context.Background()
	svc, p, _ := updateFixture(t)
	for _, version := range []string{"../1.0.0", "1.0", "1.01.0", "v1.0.0", "1.0.0-beta", "999999999999.0.0"} {
		_, err := svc.UploadRelease(ctx, UploadReleaseInput{ProductID: p.ID, Version: version, Filename: "ebox.zip"}, strings.NewReader("x"), "publisher")
		assertCode(t, err, "INVALID_REQUEST")
	}
	_, err := svc.UploadRelease(ctx, UploadReleaseInput{ProductID: p.ID, Version: "1.0.0", Filename: "ebox.zip"}, strings.NewReader(""), "publisher")
	assertCode(t, err, "INVALID_REQUEST")
	files, _ := os.ReadDir(svc.releaseDir)
	if len(files) != 0 {
		t.Fatal("failed upload left a file")
	}
	uploadTestRelease(t, svc, p, "1.0.0")
	_, err = svc.UploadRelease(ctx, UploadReleaseInput{ProductID: p.ID, Version: "1.0.0", Filename: "ebox.zip"}, strings.NewReader("x"), "publisher")
	assertCode(t, err, "RELEASE_EXISTS")
}

type releaseZeroReader struct{}

func (releaseZeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestOversizeReleaseUploadCleansTemporaryFiles(t *testing.T) {
	ctx := context.Background()
	svc, p, _ := updateFixture(t)
	_, err := svc.UploadRelease(ctx, UploadReleaseInput{ProductID: p.ID, Version: "1.0.0", Filename: "ebox.zip"}, releaseZeroReader{}, "publisher")
	assertCode(t, err, "RELEASE_TOO_LARGE")
	files, err := os.ReadDir(svc.releaseDir)
	if err != nil || len(files) != 0 {
		t.Fatalf("temporary package was not cleaned up: %v", err)
	}
}

func TestPermanentLicenseCanUpdateAndDisabledProductCannot(t *testing.T) {
	ctx := context.Background()
	svc, p, _ := updateFixture(t)
	batch, err := svc.CreateCardBatch(ctx, CreateCardBatchInput{ProductID: p.ID, Name: "Permanent", Quantity: 1, IsPermanent: true})
	if err != nil {
		t.Fatal(err)
	}
	license, err := svc.Activate(ctx, ActivateInput{AppKey: p.AppKey, CardCode: batch.Codes[0], BindMode: domain.BindDevice, BindValue: "permanent-server"})
	if err != nil {
		t.Fatal(err)
	}
	input := VerifyInput{AppKey: p.AppKey, LicenseNo: license.LicenseNo, BindMode: domain.BindDevice, BindValue: "permanent-server"}
	r := uploadTestRelease(t, svc, p, "1.0.0")
	svc.SetProductUpdates(ctx, p.ID, true, "publisher")
	svc.ChangeReleaseStatus(ctx, p.ID, r.ID, "published", "publisher")
	now := svc.now()
	svc.now = func() time.Time { return now.AddDate(10, 0, 0) }
	_, file, err := svc.DownloadRelease(ctx, r.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	if _, err = svc.ChangeProductStatus(ctx, p.ID, domain.ProductDisabled, "publisher"); err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.DownloadRelease(ctx, r.ID, input)
	assertCode(t, err, "PRODUCT_DISABLED")
}
