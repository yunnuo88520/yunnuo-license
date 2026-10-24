package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

func rightsFixture(t *testing.T, mode string, updateDays, graceDays int) (*Service, domain.Product, VerifyInput, LicenseResponse, string) {
	t.Helper()
	ctx := context.Background()
	svc := newTestService(t).WithReleaseDir(t.TempDir())
	p, err := svc.CreateProduct(ctx, CreateProductInput{Name: "Rights", Code: "RIGHT", BindMode: domain.BindDevice, LicenseMode: mode, UpdateDurationDays: updateDays, ExpireGraceDays: graceDays})
	if err != nil {
		t.Fatal(err)
	}
	card := createTestCard(t, ctx, svc, p.ID, 30)
	lic, err := svc.Activate(ctx, ActivateInput{AppKey: p.AppKey, CardCode: card, BindMode: domain.BindDevice, BindValue: "rights-device"})
	if err != nil {
		t.Fatal(err)
	}
	input := VerifyInput{AppKey: p.AppKey, LicenseNo: lic.LicenseNo, BindMode: domain.BindDevice, BindValue: "rights-device"}
	r := uploadTestRelease(t, svc, p, "2.0.0")
	if _, err = svc.SetProductUpdates(ctx, p.ID, true, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ChangeReleaseStatus(ctx, p.ID, r.ID, "published", "admin"); err != nil {
		t.Fatal(err)
	}
	return svc, p, input, lic, r.ID
}

func TestSeparateUsageAndUpdateExpiryBoundaries(t *testing.T) {
	for _, mode := range []string{domain.LicenseSubscription, domain.LicensePerpetual} {
		for _, offset := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
			t.Run(mode+"/"+offset.String(), func(t *testing.T) {
				ctx := context.Background()
				svc, _, input, issued, id := rightsFixture(t, mode, 0, 0)
				expiry := *issued.UpdatesExpiredAt
				now := expiry.Add(offset)
				svc.now = func() time.Time { return now }
				verified, err := svc.Verify(ctx, input)
				if mode == domain.LicenseSubscription && offset >= 0 {
					assertCode(t, err, "LICENSE_EXPIRED")
				} else {
					if err != nil || !verified.CanUse || verified.CanUpdate != (offset < 0) {
						t.Fatalf("wrong capabilities: %+v %v", verified, err)
					}
					if mode == domain.LicensePerpetual && (verified.UsageExpiredAt != nil || verified.ExpiredAt != nil) {
						t.Fatal("perpetual usage was given an expiry")
					}
				}
				_, checkErr := svc.CheckUpdate(ctx, CheckUpdateInput{VerifyInput: input, CurrentVersion: "1.0.0"})
				_, file, downloadErr := svc.DownloadRelease(ctx, id, input)
				if offset < 0 {
					if checkErr != nil || downloadErr != nil {
						t.Fatalf("valid entitlement denied: %v %v", checkErr, downloadErr)
					}
					file.Close()
				} else {
					code := "LICENSE_EXPIRED"
					if mode == domain.LicensePerpetual {
						code = "UPDATES_EXPIRED"
					}
					assertCode(t, checkErr, code)
					assertCode(t, downloadErr, code)
				}
			})
		}
	}
}

func TestUpdatesCanEndBeforeSubscriptionUsageAndGraceNeverExtendsUpdates(t *testing.T) {
	ctx := context.Background()
	svc, _, input, issued, _ := rightsFixture(t, domain.LicenseSubscription, 7, 3)
	start := svc.now()
	svc.now = func() time.Time { return start.AddDate(0, 0, 7) }
	v, err := svc.Verify(ctx, input)
	if err != nil || !v.CanUse || v.CanUpdate {
		t.Fatalf("seven-day update term: %+v %v", v, err)
	}
	svc.now = func() time.Time { return *issued.UsageExpiredAt }
	v, err = svc.Verify(ctx, input)
	if err != nil || !v.CanUse || v.CanUpdate {
		t.Fatalf("usage grace: %+v %v", v, err)
	}
	svc.now = func() time.Time { return issued.UsageExpiredAt.AddDate(0, 0, 3) }
	_, err = svc.Verify(ctx, input)
	assertCode(t, err, "LICENSE_EXPIRED")
}

func TestPerpetualUsageRenewsOnlyUpdatesAndOfflineCacheRemainsUsable(t *testing.T) {
	ctx := context.Background()
	svc, p, input, issued, id := rightsFixture(t, domain.LicensePerpetual, 0, 0)
	svc.now = func() time.Time { return issued.UpdatesExpiredAt.Add(time.Hour) }
	v, err := svc.Verify(ctx, input)
	if err != nil || !v.CanUse || v.CanUpdate {
		t.Fatalf("maintenance expiry stopped usage: %+v %v", v, err)
	}
	if _, err = svc.Heartbeat(ctx, input); err != nil {
		t.Fatal(err)
	}
	var claims struct {
		LicenseExpiredAt *time.Time `json:"license_expired_at"`
		OfflineUntil     time.Time  `json:"offline_until"`
		UpdatesExpiredAt *time.Time `json:"updates_expired_at"`
	}
	if err = yncrypto.VerifySignedJSON(p.PublicKeyPEM, v.OfflineToken, &claims); err != nil {
		t.Fatal(err)
	}
	if claims.LicenseExpiredAt != nil || !claims.OfflineUntil.After(svc.now()) || !claims.UpdatesExpiredAt.Equal(*issued.UpdatesExpiredAt) {
		t.Fatalf("offline usage wrongly expired: %+v", claims)
	}
	renewal := createTestCard(t, ctx, svc, p.ID, 30)
	renewed, err := svc.Renew(ctx, RenewInput{AppKey: p.AppKey, LicenseNo: input.LicenseNo, RenewCardCode: renewal, BindMode: input.BindMode, BindValue: input.BindValue})
	if err != nil || !renewed.CanUse || !renewed.CanUpdate || renewed.UsageExpiredAt != nil || !renewed.UpdatesExpiredAt.Equal(svc.now().AddDate(0, 0, 30)) {
		t.Fatalf("wrong maintenance renewal: %+v %v", renewed, err)
	}
	_, file, err := svc.DownloadRelease(ctx, id, input)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	if _, err = svc.RevokeLicense(ctx, RevokeLicenseInput{LicenseNo: input.LicenseNo}); err != nil {
		t.Fatal(err)
	}
	_, err = svc.Verify(ctx, input)
	assertCode(t, err, "LICENSE_REVOKED")
	renewal = createTestCard(t, ctx, svc, p.ID, 30)
	_, err = svc.Renew(ctx, RenewInput{AppKey: p.AppKey, LicenseNo: input.LicenseNo, RenewCardCode: renewal, BindMode: input.BindMode, BindValue: input.BindValue})
	assertCode(t, err, "LICENSE_REVOKED")
}

func TestExpiryBetweenUpdateCheckAndDownloadIsRejected(t *testing.T) {
	ctx := context.Background()
	svc, _, input, issued, id := rightsFixture(t, domain.LicensePerpetual, 0, 0)
	svc.now = func() time.Time { return issued.UpdatesExpiredAt.Add(-time.Nanosecond) }
	result, err := svc.CheckUpdate(ctx, CheckUpdateInput{VerifyInput: input, CurrentVersion: "1.0.0"})
	if err != nil || !result.UpdateAvailable {
		t.Fatal("valid check did not offer update")
	}
	svc.now = func() time.Time { return *issued.UpdatesExpiredAt }
	_, _, err = svc.DownloadRelease(ctx, id, input)
	assertCode(t, err, "UPDATES_EXPIRED")
}

func TestExpiredSubscriptionCannotReactivateWithItsOriginalCard(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	p, err := svc.CreateProduct(ctx, CreateProductInput{Name: "Subscription", Code: "SUB", BindMode: domain.BindDevice})
	if err != nil {
		t.Fatal(err)
	}
	card := createTestCard(t, ctx, svc, p.ID, 1)
	input := ActivateInput{AppKey: p.AppKey, CardCode: card, BindMode: domain.BindDevice, BindValue: "device"}
	issued, err := svc.Activate(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	svc.now = func() time.Time { return *issued.UsageExpiredAt }
	_, err = svc.Activate(ctx, input)
	assertCode(t, err, "LICENSE_EXPIRED")
	_, err = svc.Heartbeat(ctx, VerifyInput{AppKey: p.AppKey, LicenseNo: issued.LicenseNo, BindMode: domain.BindDevice, BindValue: "device"})
	assertCode(t, err, "LICENSE_EXPIRED")
}

func TestPolicyLockedAfterIssuingCardsAndRejectsInvalidTerms(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	p, err := svc.CreateProduct(ctx, CreateProductInput{Name: "Terms", Code: "TERM"})
	if err != nil {
		t.Fatal(err)
	}
	policy := LicensePolicy{LicenseMode: domain.LicensePerpetual, UpdateDurationDays: 14}
	if _, err = svc.SetLicensePolicy(ctx, p.ID, policy, "admin"); err != nil {
		t.Fatal(err)
	}
	createTestCard(t, ctx, svc, p.ID, 30)
	policy.LicenseMode = domain.LicenseSubscription
	_, err = svc.SetLicensePolicy(ctx, p.ID, policy, "admin")
	assertCode(t, err, "LICENSE_POLICY_LOCKED")
	for _, input := range []CreateProductInput{{Name: "Bad", Code: "BAD", LicenseMode: "unknown"}, {Name: "Bad", Code: "BAD", UpdateDurationDays: -1}, {Name: "Bad", Code: "BAD", ExpireGraceDays: -1}} {
		_, err = svc.CreateProduct(ctx, input)
		assertCode(t, err, "INVALID_REQUEST")
	}
}

func TestLegacyLicenseMigrationPreservesBothDeadlines(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")
	dsn := "file:" + path + "?_pragma=foreign_keys(1)"
	st, err := store.Open(ctx, "sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err = st.Migrate(ctx, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	svc := New(st, []byte("pepper"), []byte("0123456789abcdef0123456789abcdef"))
	p, err := svc.CreateProduct(ctx, CreateProductInput{Name: "Legacy", Code: "OLD", BindMode: domain.BindDevice})
	if err != nil {
		t.Fatal(err)
	}
	card := createTestCard(t, ctx, svc, p.ID, 30)
	issued, err := svc.Activate(ctx, ActivateInput{AppKey: p.AppKey, CardCode: card, BindMode: domain.BindDevice, BindValue: "old-device"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	for _, table := range []string{"license_entitlements", "product_license_policies"} {
		if _, err = raw.ExecContext(ctx, "DROP TABLE "+table); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err = st.Migrate(ctx, filepath.Join("..", "..", "migrations")); err != nil {
			t.Fatal(err)
		}
	}
	v, err := svc.Verify(ctx, VerifyInput{AppKey: p.AppKey, LicenseNo: issued.LicenseNo, BindMode: domain.BindDevice, BindValue: "old-device"})
	if err != nil || v.LicenseMode != domain.LicenseSubscription || !v.UsageExpiredAt.Equal(*issued.ExpiredAt) || !v.UpdatesExpiredAt.Equal(*issued.ExpiredAt) {
		t.Fatalf("legacy rights changed: %+v %v", v, err)
	}
}
