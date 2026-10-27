package service

import (
	"context"
	"encoding/json"
	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
	"net/netip"
	"strings"
)

func (s *Service) indexBinding(ctx context.Context, tx *store.Tx, b domain.Binding, value string) error {
	if e := tx.ClearBindingSubjects(ctx, b.ID); e != nil {
		return e
	}
	values := map[string]string{b.BindMode: value}
	if b.BindMode == "composite" {
		if e := json.Unmarshal([]byte(value), &values); e != nil {
			return e
		}
	}
	for kind, v := range values {
		if e := tx.IndexBindingSubject(ctx, b.ID, kind, yncrypto.BindHash(s.cardPepper, kind, v)); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) publicPatternBindings(ctx context.Context, kind, value string) ([]domain.Binding, error) {
	candidates := map[string][]string{}
	hash := func(k, v string) { candidates[k] = append(candidates[k], yncrypto.BindHash(s.cardPepper, k, v)) }
	hash(kind, value)
	if kind == "domain" {
		hash("domain_wildcard", value)
		labels := strings.Split(value, ".")
		for i := 0; i < len(labels)-1; i++ {
			hash("domain_wildcard", "*."+strings.Join(labels[i:], "."))
		}
	} else if kind == "ip" {
		a, e := netip.ParseAddr(value)
		if e != nil {
			return nil, e
		}
		a = a.Unmap()
		hash("ip_cidr", a.String())
		for bits := 1; bits <= a.BitLen(); bits++ {
			hash("ip_cidr", netip.PrefixFrom(a, bits).Masked().String())
		}
	}
	result := []domain.Binding{}
	seen := map[string]bool{}
	for k, hashes := range candidates {
		items, e := s.currentStore().LookupBindingSubjects(ctx, k, hashes)
		if e != nil {
			return nil, e
		}
		for _, b := range items {
			if seen[b.ID] {
				continue
			}
			registered, e := yncrypto.DecryptString(s.dataKey, b.BindValueEncrypted)
			if e != nil {
				return nil, e
			}
			l, e := s.currentStore().GetLicenseByID(ctx, b.LicenseID)
			if e != nil {
				return nil, e
			}
			t := domain.AuthorizationTerms{}
			if l.AuthorizationTerms != nil {
				t = *l.AuthorizationTerms
			}
			mode := b.BindMode
			if mode == "composite" {
				var components map[string]string
				if json.Unmarshal([]byte(registered), &components) != nil {
					return nil, ErrBindingRequired
				}
				registered = components[k]
				mode = k
			}
			if matchBinding(mode, registered, value, t) {
				seen[b.ID] = true
				result = append(result, b)
			}
		}
	}
	return result, nil
}
