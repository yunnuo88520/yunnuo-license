package store

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	core "github.com/yunnuo88520/yunnuo-license/backend/internal/licensing"
	"path/filepath"
	"testing"
	"time"
)

func scopeTestStore(t *testing.T) (*Store, core.Registry) {
	t.Helper()
	s, e := Open(context.Background(), "sqlite", "file:"+filepath.Join(t.TempDir(), "scope.sqlite")+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	if e = s.Migrate(context.Background(), "../../migrations"); e != nil {
		t.Fatal(e)
	}
	return s, s.LicensingRegistry()
}
func testAuditSigner(event core.AuditEvent) (string, error) {
	event.EventHash = ""
	b, e := json.Marshal(event)
	mac := hmac.New(sha256.New, []byte("audit-test-key"))
	mac.Write(b)
	return hex.EncodeToString(mac.Sum(nil)), e
}
func seedScopedApp(t *testing.T, r core.Registry, id string) (context.Context, core.Repository) {
	t.Helper()
	now := time.Now().UTC()
	actor := core.Actor{Kind: "admin", ID: "root", Role: "super_admin"}
	app := core.Application{Record: core.Record{ApplicationID: id, ID: id, CreatedAt: now, UpdatedAt: now}, Name: id, Status: "active", SecretHash: "hashed-only"}
	key := core.SigningKey{Record: core.Record{ApplicationID: id, ID: id + ".v1", CreatedAt: now, UpdatedAt: now}, KID: id + ".v1", Version: 1, Status: "active", PublicPEM: "public-test-key", PrivateCipher: "encrypted-only"}
	event := core.AuditEvent{Record: core.Record{CreatedAt: now, UpdatedAt: now}, Actor: actor, EventType: "created", Result: "success"}
	if e := r.CreateApplication(context.Background(), app, key, event, testAuditSigner); e != nil {
		t.Fatal(e)
	}
	ctx, e := core.BindScope(context.Background(), id, actor, true)
	if e != nil {
		t.Fatal(e)
	}
	repo, e := r.Scoped(ctx)
	if e != nil {
		t.Fatal(e)
	}
	return ctx, repo
}
func appendScopeTestEvent(ctx context.Context, tx core.Transaction, kind string) error {
	return tx.AppendAudit(ctx, core.AuditEvent{Record: core.Record{CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}, EventType: kind, Result: "success"}, testAuditSigner)
}
func TestApplicationScopeIsInjectedAndCannotBeOmitted(t *testing.T) {
	_, registry := scopeTestStore(t)
	a, ra := seedScopedApp(t, registry, "yn0000000000000001")
	b, rb := seedScopedApp(t, registry, "yn0000000000000002")
	if _, e := registry.Scoped(context.Background()); !errors.Is(e, core.ErrScopeRequired) {
		t.Fatal("missing scope allowed", e)
	}
	for _, test := range []struct {
		ctx  context.Context
		repo core.Repository
		name string
	}{{a, ra, "A"}, {b, rb, "B"}} {
		e := test.repo.Update(test.ctx, func(tx core.Transaction) error {
			if e := tx.Plans().Insert(test.ctx, core.Plan{Record: core.Record{ID: "same-plan-id"}, Name: test.name, Enabled: true, Terms: core.Terms{Model: "subscription"}}); e != nil {
				return e
			}
			return appendScopeTestEvent(test.ctx, tx, "plan_created")
		})
		if e != nil {
			t.Fatal(e)
		}
		e = test.repo.View(test.ctx, func(view core.View) error {
			rows, e := view.Plans().Find(test.ctx, core.Filter{})
			if e != nil {
				return e
			}
			if len(rows) != 1 || rows[0].Name != test.name || rows[0].ApplicationID == "" {
				t.Fatalf("unfiltered query escaped application: %#v", rows)
			}
			return nil
		})
		if e != nil {
			t.Fatal(e)
		}
	}
	if e := ra.View(context.Background(), func(core.View) error { return nil }); !errors.Is(e, core.ErrScopeRequired) {
		t.Fatal(e)
	}
	if e := ra.View(b, func(core.View) error { return nil }); !errors.Is(e, core.ErrScopeMismatch) {
		t.Fatal(e)
	}
	e := ra.Update(a, func(tx core.Transaction) error {
		return tx.Plans().Save(a, core.Plan{Record: core.Record{ApplicationID: "yn0000000000000002", ID: "same-plan-id"}, Name: "wrong"})
	})
	if !errors.Is(e, core.ErrScopeMismatch) {
		t.Fatal("foreign update allowed", e)
	}
}
func TestApplicationCompositeForeignKeysAndAuditAreMandatory(t *testing.T) {
	s, registry := scopeTestStore(t)
	a, ra := seedScopedApp(t, registry, "yn0000000000000001")
	b, rb := seedScopedApp(t, registry, "yn0000000000000002")
	e := ra.Update(a, func(tx core.Transaction) error {
		if e := tx.Plans().Insert(a, core.Plan{Record: core.Record{ID: "only-A"}, Name: "private A"}); e != nil {
			return e
		}
		return appendScopeTestEvent(a, tx, "created")
	})
	if e != nil {
		t.Fatal(e)
	}
	e = rb.Update(b, func(tx core.Transaction) error {
		if e := tx.Licenses().Insert(b, core.License{Record: core.Record{ID: "foreign-reference"}, PlanID: "only-A", Status: "active"}); e != nil {
			return e
		}
		return appendScopeTestEvent(b, tx, "created")
	})
	if e == nil {
		t.Fatal("cross-application foreign key accepted")
	}
	e = ra.Update(a, func(tx core.Transaction) error {
		return tx.Plans().Insert(a, core.Plan{Record: core.Record{ID: "unaudited"}, Name: "must roll back"})
	})
	if !errors.Is(e, core.ErrAuditRequired) {
		t.Fatal("unaudited change committed", e)
	}
	e = ra.View(a, func(view core.View) error {
		_, e := view.Plans().Get(a, "unaudited")
		if !errors.Is(e, core.ErrNotFound) {
			t.Fatal("unaudited record survived", e)
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	for _, query := range []string{"UPDATE application_audit_events SET result='modified' WHERE application_id='yn0000000000000001'", "DELETE FROM application_audit_events WHERE application_id='yn0000000000000001'"} {
		if _, e = s.db.ExecContext(context.Background(), query); e == nil {
			t.Fatal("audit mutation allowed")
		}
	}
	e = ra.View(a, func(view core.View) error {
		events, e := view.Audit().Find(a, core.Filter{Ascending: true})
		if e != nil {
			return e
		}
		previous := ""
		for i, event := range events {
			if event.Sequence != int64(i+1) || event.PreviousHash != previous {
				t.Fatal("invalid chain")
			}
			hash, e := testAuditSigner(event)
			if e != nil || event.EventHash != hash {
				t.Fatal("audit authentication mismatch", e)
			}
			previous = event.EventHash
		}
		head, e := view.AuditHead(a)
		if e != nil || head.Hash != previous {
			t.Fatal("audit head mismatch", e)
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
}

// Historical rows must not turn the read-page ceiling into an entitlement or
// invalidation bypass. This also exercises JSON/projection consistency.
func TestBulkInvalidationIncludesHistoryBeyondReadPageAndPreservesOtherApplication(t *testing.T) {
	_, registry := scopeTestStore(t)
	a, ra := seedScopedApp(t, registry, "yn0000000000000001")
	b, rb := seedScopedApp(t, registry, "yn0000000000000002")
	seed := func(ctx context.Context, repo core.Repository, n int) {
		t.Helper()
		e := repo.Update(ctx, func(tx core.Transaction) error {
			if e := tx.Plans().Insert(ctx, core.Plan{Record: core.Record{ID: "plan"}}); e != nil {
				return e
			}
			if e := tx.Licenses().Insert(ctx, core.License{Record: core.Record{ID: "license"}, PlanID: "plan", Status: "active"}); e != nil {
				return e
			}
			for i := 0; i < n; i++ {
				id := fmt.Sprintf("history-%05d", i)
				if e := tx.Anchors().Insert(ctx, core.BindingAnchor{Record: core.Record{ID: id}, LicenseID: "license", GroupID: id, BindingHash: id, Status: "active"}); e != nil {
					return e
				}
				if e := tx.Activations().Insert(ctx, core.Activation{Record: core.Record{ID: id}, LicenseID: "license", PublicKeyHash: id}); e != nil {
					return e
				}
				if e := tx.Invitations().Insert(ctx, core.Invitation{Record: core.Record{ID: id}, LicenseID: "license", TokenHash: id, PublicKeyHash: "old-key", ExpiresAt: time.Now().Add(time.Hour)}); e != nil {
					return e
				}
			}
			return appendScopeTestEvent(ctx, tx, "history_seeded")
		})
		if e != nil {
			t.Fatal(e)
		}
	}
	seed(a, ra, 10003)
	seed(b, rb, 1)
	now := time.Now().UTC().Truncate(time.Second)
	invalidate := func(tx core.Transaction) error {
		if e := tx.RetireBindings(a, "license", now); e != nil {
			return e
		}
		if e := tx.DeactivateAll(a, "license", now); e != nil {
			return e
		}
		return tx.InvalidateInvitations(a, "license", now)
	}
	if e := ra.Update(a, invalidate); !errors.Is(e, core.ErrAuditRequired) {
		t.Fatal("unaudited bulk mutation committed", e)
	}
	if e := ra.Update(a, func(tx core.Transaction) error {
		n, e := tx.CountBindingGroups(a, "license")
		if e != nil {
			return e
		}
		if n != 10003 {
			t.Fatalf("historical groups truncated: %d", n)
		}
		if e = invalidate(tx); e != nil {
			return e
		}
		return appendScopeTestEvent(a, tx, "ownership_invalidated")
	}); e != nil {
		t.Fatal(e)
	}
	for _, test := range []struct {
		ctx     context.Context
		repo    core.Repository
		foreign bool
	}{{a, ra, false}, {b, rb, true}} {
		if e := test.repo.View(test.ctx, func(v core.View) error {
			// The last record is deliberately past the maximum Find page.
			id := "history-10002"
			if test.foreign {
				id = "history-00000"
			}
			anchor, e := v.Anchors().Get(test.ctx, id)
			if e != nil {
				return e
			}
			activation, e := v.Activations().Get(test.ctx, id)
			if e != nil {
				return e
			}
			invite, e := v.Invitations().Get(test.ctx, id)
			if e != nil {
				return e
			}
			if test.foreign {
				if anchor.Status != "active" || activation.DeactivatedAt != nil || invite.UsedAt != nil || invite.PublicKeyHash != "old-key" {
					t.Fatal("cross-application invalidation")
				}
			} else if anchor.Status != "replaced" || activation.DeactivatedAt == nil || !activation.DeactivatedAt.Equal(now) || invite.UsedAt == nil || !invite.UsedAt.Equal(now) || invite.PublicKeyHash != "" {
				t.Fatal("tail history remained active or JSON diverged")
			}
			return nil
		}); e != nil {
			t.Fatal(e)
		}
	}
}
