package ynlicense

import (
	"context"
	"testing"
	"time"
)

type memoryHighWaterStore struct {
	value time.Time
}

func (s *memoryHighWaterStore) Load(context.Context) (time.Time, error) {
	return s.value, nil
}

func (s *memoryHighWaterStore) Save(_ context.Context, value time.Time) error {
	s.value = value
	return nil
}

func TestHighWaterGuard(t *testing.T) {
	ctx := context.Background()
	store := &memoryHighWaterStore{}
	guard, err := NewHighWaterGuard(store, time.Minute)
	if err != nil {
		t.Fatalf("new guard: %v", err)
	}
	base := time.Date(2026, 8, 10, 3, 0, 0, 0, time.UTC)
	if err := guard.CheckAndUpdate(ctx, base); err != nil {
		t.Fatalf("first update: %v", err)
	}
	if err := guard.CheckAndUpdate(ctx, base.Add(-30*time.Second)); err != nil {
		t.Fatalf("small clock adjustment should be tolerated: %v", err)
	}
	if err := guard.CheckAndUpdate(ctx, base.Add(-2*time.Minute)); !IsVerificationErrorCode(err, VerificationClockRollback) {
		t.Fatalf("expected clock rollback, got %v", err)
	}
	if err := guard.CheckAndUpdate(ctx, base.Add(time.Hour)); err != nil {
		t.Fatalf("advance high water: %v", err)
	}
	if !store.value.Equal(base.Add(time.Hour)) {
		t.Fatalf("unexpected high-water value: %v", store.value)
	}
}
