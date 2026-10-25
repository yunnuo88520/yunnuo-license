package service

import (
	"context"
	"testing"
	"time"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
)

func TestBindingOnlineStates(t *testing.T) {
	now := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)
	heartbeats := func(d time.Duration) *time.Time {
		at := now.Add(-d)
		return &at
	}
	bindings := []domain.Binding{
		{ID: "fresh", Status: domain.BindingActive, LastHeartbeatAt: heartbeats(2 * time.Hour), ActivatedAt: now.Add(-48 * time.Hour)},
		{ID: "idle", Status: domain.BindingActive, LastHeartbeatAt: heartbeats(72 * time.Hour), ActivatedAt: now.Add(-96 * time.Hour)},
		{ID: "lost", Status: domain.BindingActive, LastHeartbeatAt: heartbeats(40 * 24 * time.Hour), ActivatedAt: now.Add(-60 * 24 * time.Hour)},
		{ID: "never", Status: domain.BindingActive, ActivatedAt: now.Add(-2 * time.Hour)},
		{ID: "unbound", Status: domain.BindingUnbound, LastHeartbeatAt: heartbeats(1 * time.Hour)},
	}
	bindingOnlineStates(bindings, 15, now)

	expect := map[string]struct {
		state string
		days  int
	}{
		"fresh":   {"online", 0},
		"idle":    {"idle", 3},
		"lost":    {"offline", 40},
		"never":   {"online", 0},
		"unbound": {"", 0},
	}
	for _, b := range bindings {
		want, ok := expect[b.ID]
		if !ok {
			t.Fatalf("unexpected binding %s", b.ID)
		}
		if b.OnlineState != want.state || b.OfflineDays != want.days {
			t.Fatalf("%s: got state=%s days=%d, want state=%s days=%d", b.ID, b.OnlineState, b.OfflineDays, want.state, want.days)
		}
		if b.Status == domain.BindingActive && b.LastSeenAt == nil {
			t.Fatalf("%s: active binding should expose last_seen_at", b.ID)
		}
		if b.Status != domain.BindingActive && (b.OnlineState != "" || b.LastSeenAt != nil) {
			t.Fatalf("%s: inactive binding must not carry online state", b.ID)
		}
	}
}

func TestListLicenseBindingsExposesOnlineState(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	product := createTestProduct(t, ctx, svc, domain.ConflictReject)
	code := createTestCard(t, ctx, svc, product.ID, 30)
	resp, err := svc.Activate(ctx, ActivateInput{
		AppKey: product.AppKey, CardCode: code, BindMode: domain.BindDevice, BindValue: "state-device",
	})
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	// 激活后立即查看：处于 24 小时窗口内，应为 online。
	bindings, err := svc.ListLicenseBindings(ctx, resp.LicenseNo)
	if err != nil || len(bindings) != 1 {
		t.Fatalf("list bindings: %v", err)
	}
	if bindings[0].OnlineState != "online" || bindings[0].LastSeenAt == nil {
		t.Fatalf("fresh binding should be online: %#v", bindings[0])
	}

	// 推进时间到 20 天后：超过 15 天离线容忍期，应为 offline。
	base := svc.now()
	svc.now = func() time.Time { return base.Add(20 * 24 * time.Hour) }
	bindings, err = svc.ListLicenseBindings(ctx, resp.LicenseNo)
	if err != nil {
		t.Fatal(err)
	}
	if bindings[0].OnlineState != "offline" || bindings[0].OfflineDays < 19 {
		t.Fatalf("stale binding should be offline: %#v", bindings[0])
	}
}
