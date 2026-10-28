package licensing_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	core "github.com/yunnuo88520/yunnuo-license/backend/internal/licensing"
	"io"
	"testing"
	"time"
)

func enableUpdates(t *testing.T, s *core.Service, root context.Context, app core.CreatedApplication) core.Application {
	t.Helper()
	enabled := true
	out, e := s.SaveApplication(root, app.AppID, core.CreateApplicationInput{Name: app.Application.Name, AllowedModels: app.Application.AllowedModels, AllowedBindings: app.Application.AllowedBindings, Forms: app.Application.Forms, VerifyPolicy: app.Application.VerifyPolicy, UpdatesEnabled: &enabled}, "active")
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func publish(t *testing.T, s *core.Service, root context.Context, app string, version string, target core.ReleaseTarget) core.Release {
	t.Helper()
	r, e := s.UploadRelease(root, app, core.UploadInput{Version: version, Filename: "product.tar.gz", Target: target}, bytes.NewBufferString("same generic package "+version))
	if e != nil {
		t.Fatal(e)
	}
	r, e = s.SetReleaseStatus(root, app, r.ID, "published", "acceptance release")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.AuditIntegrity(root, app); e != nil {
		t.Fatal("published release audit must survive JSON round trip", e)
	}
	return r
}
func TestUpdateDeadlinesAreStrictAtCheckDownloadAndInstall(t *testing.T) {
	s, _, root, now := coreFixture(t)
	s.WithReleaseStorage(t.TempDir(), core.StorageLimits{})
	app := fullApplication(t, s, root)
	enableUpdates(t, s, root, app)
	sale := issue(t, s, root, app.AppID, core.Terms{Model: "perpetual", BindingTypes: []string{"installation"}, UpdateDurationDays: 1}, "")
	client := newInstall(t)
	activated, e := activate(t, s, client, *now, core.ActivateInput{AppID: app.AppID, CardCode: sale.Cards[0].Code, Binding: core.BindingInput{Type: "installation", Value: "stable-host"}, Form: "gui"})
	if e != nil {
		t.Fatal(e)
	}
	target := core.ReleaseTarget{OS: "linux", Arch: "amd64", Runtime: "python", Channel: "stable", Kind: "archive"}
	r := publish(t, s, root, app.AppID, "2.0.0", target)
	in := core.UpdateInput{AppID: app.AppID, ActivationID: activated.ActivationID, Binding: core.BindingInput{Type: "installation", Value: "stable-host"}, Form: "gui", CurrentVersion: "1.0.0", Target: target}
	check := func() (core.UpdateResult, error) {
		return s.CheckUpdate(context.Background(), in, client.request(t, *now, "/v2/updates/check", app.AppID, in))
	}
	found, e := check()
	if e != nil || !found.Available || found.Release.ID != r.ID {
		t.Fatal(found, e)
	}
	if _, e = s.RotateSigningKey(root, app.AppID, "published artifacts survive rotation", true); e != nil {
		t.Fatal(e)
	}
	found, e = check()
	if e != nil || found.ManifestJWT == "" {
		t.Fatal("manifest not re-signed with current key", e)
	}
	result, file, e := s.DownloadUpdate(context.Background(), in, r.ID, client.request(t, *now, "/v2/updates/"+r.ID+"/download", app.AppID, in))
	if e != nil {
		t.Fatal(e)
	}
	raw, e := io.ReadAll(file)
	file.Close()
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != result.Release.SHA256 {
		t.Fatal("package hash mismatch")
	}
	*now = now.Add(24*time.Hour - time.Second)
	if _, e = check(); e != nil {
		t.Fatal("premature update expiry", e)
	}
	*now = now.Add(time.Second)
	_, e = check()
	assertCode(t, e, "UPDATES_EXPIRED")
	_, file, e = s.DownloadUpdate(context.Background(), in, r.ID, client.request(t, *now, "/v2/updates/"+r.ID+"/download", app.AppID, in))
	assertCode(t, e, "UPDATES_EXPIRED")
	if file != nil {
		t.Fatal("expired download opened package")
	}
	_, e = s.AuthorizeUpdate(context.Background(), in, r.ID, client.request(t, *now, "/v2/updates/"+r.ID+"/authorize", app.AppID, in))
	assertCode(t, e, "UPDATES_EXPIRED")
	// Update expiry does not take away independently purchased perpetual use.
	if _, e = verify(t, s, client, *now, core.VerifyInput{AppID: app.AppID, ActivationID: activated.ActivationID, Binding: in.Binding, Form: "gui"}); e != nil {
		t.Fatal("perpetual use stopped on update expiry", e)
	}
}
func TestSubscriptionGraceDoesNotExtendUpdatesAndTargetsCannotCross(t *testing.T) {
	s, _, root, now := coreFixture(t)
	s.WithReleaseStorage(t.TempDir(), core.StorageLimits{})
	app := fullApplication(t, s, root)
	enabled := true
	p := app.Application.VerifyPolicy
	p.SubscriptionGraceSeconds = 3 * 86400
	if _, e := s.SaveApplication(root, app.AppID, core.CreateApplicationInput{Name: "Grace app", AllowedModels: app.Application.AllowedModels, AllowedBindings: app.Application.AllowedBindings, Forms: app.Application.Forms, VerifyPolicy: p, UpdatesEnabled: &enabled}, "active"); e != nil {
		t.Fatal(e)
	}
	sale := issue(t, s, root, app.AppID, core.Terms{Model: "subscription", BindingTypes: []string{"installation"}, DurationDays: 1, UpdateDurationDays: 10}, "")
	client := newInstall(t)
	a, e := activate(t, s, client, *now, core.ActivateInput{AppID: app.AppID, CardCode: sale.Cards[0].Code, Binding: core.BindingInput{Type: "installation", Value: "same-package"}, Form: "container"})
	if e != nil {
		t.Fatal(e)
	}
	linux := publish(t, s, root, app.AppID, "2.0.0", core.ReleaseTarget{OS: "linux", Arch: "amd64", Runtime: "node", Kind: "archive"})
	publish(t, s, root, app.AppID, "9.0.0", core.ReleaseTarget{OS: "windows", Arch: "arm64", Runtime: "node", Kind: "installer"})
	in := core.UpdateInput{AppID: app.AppID, ActivationID: a.ActivationID, Binding: core.BindingInput{Type: "installation", Value: "same-package"}, Form: "container", CurrentVersion: "1.0.0", Target: core.ReleaseTarget{OS: "linux", Arch: "amd64", Runtime: "node", Channel: "stable", Kind: "archive"}}
	out, e := s.CheckUpdate(context.Background(), in, client.request(t, *now, "/v2/updates/check", app.AppID, in))
	if e != nil || out.Release == nil || out.Release.ID != linux.ID {
		t.Fatal("incorrect platform selected", out, e)
	}
	if _, e = s.UploadRelease(root, app.AppID, core.UploadInput{Version: "2.0.0", Filename: "replace.zip", Target: linux.Target}, bytes.NewBufferString("replacement")); e == nil {
		t.Fatal("published artifact overwritten")
	}
	if _, e = s.SetReleaseStatus(root, app.AppID, linux.ID, "withdrawn", "test withdrawal"); e != nil {
		t.Fatal(e)
	}
	out, e = s.CheckUpdate(context.Background(), in, client.request(t, *now, "/v2/updates/check", app.AppID, in))
	if e != nil || out.Available {
		t.Fatal("withdrawn version offered", out, e)
	}
	*now = now.Add(24 * time.Hour)
	if _, e = verify(t, s, client, *now, core.VerifyInput{AppID: app.AppID, ActivationID: a.ActivationID, Binding: in.Binding, Form: in.Form}); e != nil {
		t.Fatal("explicit use grace not respected", e)
	}
	_, e = s.CheckUpdate(context.Background(), in, client.request(t, *now, "/v2/updates/check", app.AppID, in))
	assertCode(t, e, "UPDATES_EXPIRED")
}
