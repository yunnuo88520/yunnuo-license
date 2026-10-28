package licensing

import (
	"context"
	"sort"
	"time"
)

// Rebuild from the original paid terms and the remaining redeemed credits.
// Subtracting days or restoring an old date is wrong when a later paid renewal
// happened after an expiry gap. Card metadata is the application-scoped ledger.
func (s *Service) recomputeRenewals(ctx context.Context, tx Transaction, id string) error {
	l, e := tx.Licenses().Get(ctx, id)
	if e != nil {
		return e
	}
	cards := []CardKey{}
	for offset := 0; ; offset += 1000 {
		page, e := tx.Cards().Find(ctx, Filter{RedeemedLicenseID: id, Limit: 1000, Offset: offset})
		if e != nil {
			return e
		}
		cards = append(cards, page...)
		if len(page) < 1000 {
			break
		}
	}
	sort.Slice(cards, func(i, j int) bool {
		a, b := cards[i].UsedAt, cards[j].UsedAt
		if a == nil {
			return false
		}
		if b == nil {
			return true
		}
		if a.Equal(*b) {
			return cards[i].ID < cards[j].ID
		}
		return a.Before(*b)
	})
	l.Terms.DurationDays = l.BaseDurationDays
	l.Terms.UpdateDurationDays = l.BaseUpdateDays
	l.ExpiresAt = l.BaseExpiresAt
	l.UpdatesUntil = l.BaseUpdatesUntil
	add := func(until *time.Time, at time.Time, days int) *time.Time {
		if until != nil && until.After(at) {
			at = *until
		}
		value := at.AddDate(0, 0, days)
		return &value
	}
	for _, card := range cards {
		if card.UsedAt == nil || card.RefundedAt != nil {
			continue
		}
		source, e := tx.Licenses().Get(ctx, card.BoundLicenseID)
		if e != nil {
			return e
		}
		at := *card.UsedAt
		beforeActivation := l.ActivatedAt == nil || at.Before(*l.ActivatedAt)
		if l.ActivatedAt != nil && beforeActivation {
			at = *l.ActivatedAt
		}
		switch card.Purpose {
		case "renew_usage":
			if beforeActivation {
				l.Terms.DurationDays += source.Terms.DurationDays
			}
			if l.ActivatedAt != nil {
				l.ExpiresAt = add(l.ExpiresAt, at, source.Terms.DurationDays)
			}
		case "renew_updates":
			if beforeActivation {
				l.Terms.UpdateDurationDays += source.Terms.UpdateDurationDays
			}
			if l.ActivatedAt != nil {
				l.UpdatesUntil = add(l.UpdatesUntil, at, source.Terms.UpdateDurationDays)
			}
		}
	}
	l.UpdatedAt = s.now()
	if e = tx.Licenses().Save(ctx, l); e != nil {
		return e
	}
	return s.append(ctx, tx, "renewal_refund_reconciled", id, map[string]any{"usage_expires_at": l.ExpiresAt, "updates_until": l.UpdatesUntil})
}
