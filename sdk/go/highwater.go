package ynlicense

import (
	"context"
	"errors"
	"sync"
	"time"
)

type HighWaterStore interface {
	Load(ctx context.Context) (time.Time, error)
	Save(ctx context.Context, value time.Time) error
}

type HighWaterGuard struct {
	store           HighWaterStore
	allowedRollback time.Duration
	mu              sync.Mutex
}

func NewHighWaterGuard(store HighWaterStore, allowedRollback time.Duration) (*HighWaterGuard, error) {
	if store == nil {
		return nil, errors.New("ynlicense: high-water store is required")
	}
	if allowedRollback < 0 {
		return nil, errors.New("ynlicense: allowed rollback cannot be negative")
	}
	return &HighWaterGuard{store: store, allowedRollback: allowedRollback}, nil
}

func (g *HighWaterGuard) CheckAndUpdate(ctx context.Context, current time.Time) error {
	if current.IsZero() {
		return errors.New("ynlicense: current time is required")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	current = current.UTC()
	last, err := g.store.Load(ctx)
	if err != nil {
		return err
	}
	if !last.IsZero() {
		last = last.UTC()
		if current.Add(g.allowedRollback).Before(last) {
			return verificationError(VerificationClockRollback, "current time is earlier than the saved high-water time")
		}
		if !current.After(last) {
			return nil
		}
	}
	return g.store.Save(ctx, current)
}
