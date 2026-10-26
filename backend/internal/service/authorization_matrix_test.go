package service

import (
	"context"
	"fmt"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"testing"
	"time"
)

func TestAuthorizationAndUpdateDeadlineMatrixForEveryBindingMode(t *testing.T) {
	bindings := []struct{ mode, value string }{
		{domain.BindNone, ""}, {domain.BindDevice, "matrix-device"}, {domain.BindDomain, "https://MATRIX.example.com/path"},
		{domain.BindIP, "192.0.2.40"}, {domain.BindIP, "2001:db8::40"}, {domain.BindAccount, "Buyer@Example.com"},
	}
	for _, mode := range []string{domain.LicenseSubscription, domain.LicensePerpetual} {
		for _, binding := range bindings {
			t.Run(mode+"/"+binding.mode+"/"+binding.value, func(t *testing.T) {
				ctx := context.Background()
				s := newTestService(t).WithReleaseDir(t.TempDir())
				start := s.now()
				p, err := s.CreateProduct(ctx, CreateProductInput{Name: "Deadline matrix", LicenseMode: mode, BindMode: binding.mode, UpdateDurationDays: 7, ExpireGraceDays: 3})
				if err != nil {
					t.Fatal(err)
				}
				card := createTestCard(t, ctx, s, p.ID, 30)
				lic, err := s.Activate(ctx, ActivateInput{AppKey: p.AppKey, CardCode: card, BindMode: binding.mode, BindValue: binding.value})
				if err != nil {
					t.Fatal(err)
				}
				input := VerifyInput{AppKey: p.AppKey, LicenseNo: lic.LicenseNo, BindMode: binding.mode, BindValue: binding.value}
				release := uploadTestRelease(t, s, p, "2.0.0")
				s.SetProductUpdates(ctx, p.ID, true, "admin")
				s.ChangeReleaseStatus(ctx, p.ID, release.ID, "published", "admin")
				updateDeadline := start.AddDate(0, 0, 7)
				usageDeadline := start.AddDate(0, 0, 30)
				graceDeadline := start.AddDate(0, 0, 33)
				instants := []time.Time{start, updateDeadline.Add(-time.Nanosecond), updateDeadline, updateDeadline.Add(time.Nanosecond), usageDeadline.Add(-time.Nanosecond), usageDeadline, usageDeadline.Add(time.Nanosecond), graceDeadline.Add(-time.Nanosecond), graceDeadline, graceDeadline.Add(time.Nanosecond), start.AddDate(1, 0, 0)}
				for _, instant := range instants {
					t.Run(instant.Format(time.RFC3339Nano), func(t *testing.T) {
						s.now = func() time.Time { return instant }
						canUse := mode == domain.LicensePerpetual || instant.Before(graceDeadline)
						canUpdate := instant.Before(updateDeadline)
						verified, e := s.Verify(ctx, input)
						if !canUse {
							assertCode(t, e, "LICENSE_EXPIRED")
						} else if e != nil || !verified.CanUse || verified.CanUpdate != canUpdate {
							t.Fatalf("wrong rights %+v %v", verified, e)
						}
						_, heartbeatErr := s.Heartbeat(ctx, input)
						if canUse && heartbeatErr != nil {
							t.Fatal(heartbeatErr)
						}
						if !canUse {
							assertCode(t, heartbeatErr, "LICENSE_EXPIRED")
						}
						_, checkErr := s.CheckUpdate(ctx, CheckUpdateInput{VerifyInput: input, CurrentVersion: "1.0.0"})
						_, authorizeErr := s.AuthorizeRelease(ctx, release.ID, input)
						_, file, downloadErr := s.DownloadRelease(ctx, release.ID, input)
						if file != nil {
							file.Close()
						}
						if canUpdate {
							if checkErr != nil || downloadErr != nil || authorizeErr != nil {
								t.Fatal(checkErr, downloadErr, authorizeErr)
							}
						} else {
							code := "UPDATES_EXPIRED"
							if mode == domain.LicenseSubscription && !instant.Before(usageDeadline) {
								code = "LICENSE_EXPIRED"
							}
							assertCode(t, checkErr, code)
							assertCode(t, downloadErr, code)
							assertCode(t, authorizeErr, code)
							if file != nil {
								t.Fatal("expired entitlement opened a downloadable package")
							}
						}
						if mode == domain.LicensePerpetual && canUse && verified.UsageExpiredAt != nil {
							t.Fatal("perpetual usage was given a deadline")
						}
					})
				}
			})
		}
	}
}

func TestUpdateRevocationWithdrawalDisableAndBindingMatrix(t *testing.T) {
	for _, action := range []string{"revoke", "withdraw", "updates_disabled", "product_disabled", "wrong_binding", "other_product"} {
		t.Run(action, func(t *testing.T) {
			ctx := context.Background()
			s, p, input, _, release := rightsFixture(t, domain.LicensePerpetual, 0, 0)
			_, e := s.CheckUpdate(ctx, CheckUpdateInput{VerifyInput: input, CurrentVersion: "1.0.0"})
			if e != nil {
				t.Fatal(e)
			}
			switch action {
			case "revoke":
				s.RevokeLicense(ctx, RevokeLicenseInput{LicenseNo: input.LicenseNo, Reason: "Matrix revoke"})
			case "withdraw":
				s.ChangeReleaseStatus(ctx, p.ID, release, "withdrawn", "admin")
			case "updates_disabled":
				s.SetProductUpdates(ctx, p.ID, false, "admin")
			case "product_disabled":
				s.ChangeProductStatus(ctx, p.ID, domain.ProductDisabled, "admin")
			case "wrong_binding":
				input.BindValue = "other-instance"
			case "other_product":
				other := createTestProduct(t, ctx, s, domain.ConflictReject)
				input.AppKey = other.AppKey
			}
			if _, err := s.AuthorizeRelease(ctx, release, input); err == nil {
				t.Fatal("changed state authorized installation")
			}
			_, file, err := s.DownloadRelease(ctx, release, input)
			if file != nil {
				file.Close()
				t.Fatal("changed authorization state yielded bytes")
			}
			if err == nil {
				t.Fatal("changed authorization state accepted download")
			}
		})
	}
}

func TestRenewalRetriesPreserveExactDeadlinesIncludingExpiredAndPermanentRights(t *testing.T) {
	for _, mode := range []string{domain.LicenseSubscription, domain.LicensePerpetual} {
		for _, permanent := range []bool{false, true} {
			for _, day := range []int{15, 45} {
				t.Run(mode+"/"+fmt.Sprint(permanent)+"/"+fmt.Sprint(day), func(t *testing.T) {
					ctx := context.Background()
					s := newTestService(t)
					start := s.now()
					p, err := s.CreateProduct(ctx, CreateProductInput{Name: "Renewal retry", BindMode: domain.BindDevice, LicenseMode: mode})
					if err != nil {
						t.Fatal(err)
					}
					initial := createTestCard(t, ctx, s, p.ID, 30)
					lic, err := s.Activate(ctx, ActivateInput{AppKey: p.ID, CardCode: initial, BindMode: domain.BindDevice, BindValue: "retry-device"})
					if err != nil {
						t.Fatal(err)
					}
					batch, err := s.CreateCardBatch(ctx, CreateCardBatchInput{ProductID: p.ID, Name: "Renewal", Quantity: 1, DurationDays: 30, IsPermanent: permanent})
					if err != nil {
						t.Fatal(err)
					}
					now := start.AddDate(0, 0, day)
					s.now = func() time.Time { return now }
					input := RenewInput{AppKey: p.ID, LicenseNo: lic.LicenseNo, BindMode: domain.BindDevice, BindValue: "retry-device", RenewCardCode: batch.Codes[0]}
					renewed, err := s.Renew(ctx, input)
					if err != nil {
						t.Fatal(err)
					}
					if permanent {
						if renewed.UpdatesExpiredAt != nil || renewed.UsageExpiredAt != nil {
							t.Fatal("permanent card lost rights")
						}
					} else {
						want := start.AddDate(0, 0, max(day, 30)+30)
						if renewed.UpdatesExpiredAt == nil || !renewed.UpdatesExpiredAt.Equal(want) {
							t.Fatal("wrong renewal start", renewed.UpdatesExpiredAt, want)
						}
					}
					for i := 0; i < 3; i++ {
						again, err := s.Renew(ctx, input)
						if err != nil {
							t.Fatal(err)
						}
						if !sameDeadline(renewed.UsageExpiredAt, again.UsageExpiredAt) || !sameDeadline(renewed.UpdatesExpiredAt, again.UpdatesExpiredAt) {
							t.Fatal("card replay extended rights")
						}
					}
					input.BindValue = "wrong-device"
					if _, err = s.Renew(ctx, input); err == nil {
						t.Fatal("renewal replay bypassed binding")
					}
				})
			}
		}
	}
}
func sameDeadline(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}
