package service

import (
	"context"
	"database/sql"
	"encoding/json"
	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
	"net/netip"
	"strings"
)

func validBindingMode(mode string) bool {
	switch mode {
	case "none", "device", "domain", "domain_wildcard", "ip", "ip_cidr", "account", "tenant", "instance", "cluster", "composite":
		return true
	}
	return false
}

func (s *Service) CardRequirements(ctx context.Context, app, code string) (map[string]any, error) {
	p, e := s.productByAppKey(ctx, app)
	if e != nil {
		return nil, e
	}
	c, e := s.currentStore().GetCardByHash(ctx, yncrypto.CardHash(s.cardPepper, code))
	if e != nil || c.ProductID != p.ID {
		return nil, ErrCardInvalid
	}
	if c.Status != domain.CardUnused && c.Status != domain.CardActivated {
		return nil, ErrCardInvalid
	}
	t := domain.LegacyTerms(effectiveProduct(p, c.AuthorizationTerms))
	return map[string]any{"appid": p.ID, "name": p.Name, "authorization_terms": t, "duration_days": c.DurationDays, "is_permanent": c.IsPermanent}, nil
}
func normalizeTerms(t domain.AuthorizationTerms) (domain.AuthorizationTerms, error) {
	t.Version = 2
	if t.CardPurpose == "" {
		t.CardPurpose = "license"
	}
	if t.CardPurpose != "license" && t.CardPurpose != "renew_usage" && t.CardPurpose != "renew_updates" {
		return t, ErrInvalidRequest
	}
	if t.BindMode == "" {
		t.BindMode = "device"
	}
	if !validBindingMode(t.BindMode) {
		return t, badRequest("TERMS_INVALID", "请选择支持的绑定方式")
	}
	if !validLicensePolicy(t.LicenseMode, t.UpdateDurationDays, t.ExpireGraceDays) {
		return t, ErrInvalidRequest
	}
	if t.MaxBindings == 0 {
		t.MaxBindings = 1
	}
	if t.MaxDeployments == 0 {
		t.MaxDeployments = 1
	}
	if t.SeatLeaseSeconds == 0 {
		t.SeatLeaseSeconds = 120
	}
	if t.SubdomainDepth == "" {
		t.SubdomainDepth = "one"
	}
	if t.MaxBindings < 1 || t.MaxBindings > 1000 || t.MaxDeployments < 1 || t.MaxDeployments > 10000 || t.MaxDevices < 0 || t.MaxDevices > 10000 || t.MaxConcurrentSeats < 0 || t.MaxConcurrentSeats > 10000 || t.SeatLeaseSeconds < 30 || t.SeatLeaseSeconds > 3600 || t.OfflineGraceDays < 0 || t.OfflineGraceDays > 365 || t.UnbindLimit < -1 || t.UnbindLimit > 10000 || t.UnbindCooldownHours < 0 || t.UnbindCooldownHours > 87600 || (t.SubdomainDepth != "one" && t.SubdomainDepth != "all") {
		return t, ErrInvalidRequest
	}
	if t.MaxDevices > 0 || t.MaxConcurrentSeats > 0 || t.BindMode == "instance" || t.BindMode == "cluster" {
		t.RequireInstance = true
	}
	if len(t.Forms) == 0 {
		t.Forms = []string{"service"}
	}
	if len(t.Forms) > 12 || len(t.Features) > 100 {
		return t, ErrInvalidRequest
	}
	forms := map[string]bool{}
	for _, f := range t.Forms {
		switch f {
		case "web", "container", "cluster", "gui", "cli", "service", "plugin", "mobile", "iot", "saas":
		default:
			return t, ErrInvalidRequest
		}
		if forms[f] {
			return t, ErrInvalidRequest
		}
		forms[f] = true
	}
	features := map[string]bool{}
	for _, f := range t.Features {
		if len(f) < 1 || len(f) > 64 || strings.TrimSpace(f) != f || features[f] {
			return t, ErrInvalidRequest
		}
		features[f] = true
	}
	if t.BindMode == "composite" {
		if len(t.BindingKinds) < 2 || len(t.BindingKinds) > 6 {
			return t, ErrInvalidRequest
		}
		seen := map[string]bool{}
		for _, k := range t.BindingKinds {
			if !validBindingMode(k) || k == "none" || k == "composite" || seen[k] {
				return t, ErrInvalidRequest
			}
			seen[k] = true
		}
	} else if len(t.BindingKinds) > 0 {
		return t, ErrInvalidRequest
	}
	return t, nil
}
func (s *Service) AuthorizationTerms(ctx context.Context, id string) (domain.AuthorizationTerms, error) {
	p, e := s.currentStore().GetProduct(ctx, id)
	if e != nil {
		return domain.AuthorizationTerms{}, ErrProductNotFound
	}
	return domain.LegacyTerms(p), nil
}

func (s *Service) ProductPackages(ctx context.Context, id string) ([]map[string]any, error) {
	packages, e := s.currentStore().Packages(ctx)
	if e != nil {
		return nil, e
	}
	items := []map[string]any{}
	for _, p := range packages {
		if p.ProductID == id && p.Enabled {
			items = append(items, map[string]any{"id": p.ID, "name": p.Name, "duration_days": p.DurationDays, "is_permanent": p.IsPermanent, "authorization_terms": p.AuthorizationTerms})
		}
	}
	return items, nil
}
func (s *Service) SetAuthorizationTerms(ctx context.Context, id, actor string, input domain.AuthorizationTerms) (domain.AuthorizationTerms, error) {
	t, e := normalizeTerms(input)
	if e != nil {
		return t, e
	}
	if _, e = s.currentStore().GetProduct(ctx, id); e != nil {
		return t, ErrProductNotFound
	}
	e = s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if e := tx.LockProduct(ctx, id); e != nil {
			return e
		}
		if e := tx.SaveTerms(ctx, "product", id, &t); e != nil {
			return e
		}
		return s.transactionAudit(ctx, tx, "admin", actor, id, "", "authorization.defaults", t)
	})
	return t, e
}
func effectiveProduct(p domain.Product, t *domain.AuthorizationTerms) domain.Product {
	p.AuthorizationTerms = nil
	if t != nil {
		return t.Apply(p)
	}
	return p
}
func (s *Service) productForLicense(ctx context.Context, p domain.Product, no string) domain.Product {
	l, e := s.currentStore().GetLicenseByNo(ctx, no)
	if e == nil && l.ProductID == p.ID {
		return effectiveProduct(p, l.AuthorizationTerms)
	}
	return effectiveProduct(p, nil)
}
func (s *Service) productForCard(ctx context.Context, p domain.Product, code string) domain.Product {
	c, e := s.currentStore().GetCardByHash(ctx, yncrypto.CardHash(s.cardPepper, code))
	if e == nil && c.ProductID == p.ID {
		return effectiveProduct(p, c.AuthorizationTerms)
	}
	return effectiveProduct(p, nil)
}

func canonicalDomain(value string, pattern bool) (string, error) {
	value = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
	wild := strings.HasPrefix(value, "*.")
	if wild {
		if !pattern {
			return "", ErrBindingRequired
		}
		value = strings.TrimPrefix(value, "*.")
	}
	ascii, e := idna.Lookup.ToASCII(value)
	if e != nil || !isValidDomain(ascii) {
		return "", ErrBindingRequired
	}
	if wild {
		if _, e = publicsuffix.EffectiveTLDPlusOne(ascii); e != nil {
			return "", ErrBindingRequired
		}
		return "*." + ascii, nil
	}
	return ascii, nil
}
func normalizeExtendedBinding(mode, value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 2000 {
		return "", ErrBindingRequired
	}
	switch mode {
	case "domain_wildcard":
		return canonicalDomain(value, true)
	case "ip_cidr":
		if a, e := netip.ParseAddr(value); e == nil {
			return a.Unmap().String(), nil
		}
		p, e := netip.ParsePrefix(value)
		if e != nil || p.Bits() == 0 || p.Addr().Zone() != "" {
			return "", ErrBindingRequired
		}
		return p.Masked().String(), nil
	case "instance", "tenant", "cluster":
		if len(value) > 128 || strings.ContainsAny(value, "\x00\r\n") {
			return "", ErrBindingRequired
		}
		return value, nil
	case "composite":
		var values map[string]string
		if json.Unmarshal([]byte(value), &values) != nil || len(values) < 2 || len(values) > 6 {
			return "", ErrBindingRequired
		}
		for k, v := range values {
			if !validBindingMode(k) || k == "composite" || k == "none" {
				return "", ErrBindingRequired
			}
			n, e := normalizeBindingValue(k, v)
			if e != nil {
				return "", e
			}
			values[k] = n
		}
		raw, _ := json.Marshal(values)
		return string(raw), nil
	}
	return "", ErrBindingRequired
}
func bindingInput(value string, values map[string]string) string {
	if len(values) == 0 {
		return value
	}
	if value != "" {
		return ""
	}
	b, _ := json.Marshal(values)
	return string(b)
}
func validateBindingShape(p domain.Product, value string) error {
	if p.BindMode != "composite" {
		return nil
	}
	var m map[string]string
	if json.Unmarshal([]byte(value), &m) != nil {
		return ErrBindingRequired
	}
	t := domain.LegacyTerms(p)
	if len(m) != len(t.BindingKinds) {
		return ErrBindingMismatch
	}
	for _, k := range t.BindingKinds {
		if _, ok := m[k]; !ok {
			return ErrBindingMismatch
		}
	}
	return nil
}
func matchBinding(mode, registered, actual string, t domain.AuthorizationTerms) bool {
	switch mode {
	case "domain_wildcard":
		if strings.Contains(actual, "*") {
			return false
		}
		if !strings.HasPrefix(registered, "*.") {
			return registered == actual
		}
		base := strings.TrimPrefix(registered, "*.")
		if actual == base {
			return t.IncludeApex
		}
		if !strings.HasSuffix(actual, "."+base) {
			return false
		}
		prefix := strings.TrimSuffix(actual, "."+base)
		return prefix != "" && (t.SubdomainDepth == "all" || !strings.Contains(prefix, "."))
	case "ip_cidr":
		p, e := netip.ParsePrefix(registered)
		a, ae := netip.ParseAddr(actual)
		if ae != nil {
			return false
		}
		if e != nil {
			return registered == a.Unmap().String()
		}
		return p.Contains(a.Unmap())
	case "composite":
		var r, a map[string]string
		if json.Unmarshal([]byte(registered), &r) != nil || json.Unmarshal([]byte(actual), &a) != nil || len(r) != len(a) {
			return false
		}
		for k, v := range r {
			av, ok := a[k]
			if !ok || !matchBinding(k, v, av, t) {
				return false
			}
		}
		return true
	}
	return registered == actual
}
func (s *Service) authorizedBinding(ctx context.Context, tx *store.Tx, p domain.Product, license, mode, value, hash string) (domain.Binding, error) {
	if mode != "domain_wildcard" && mode != "ip_cidr" && mode != "composite" {
		return tx.ActiveBindingByHash(ctx, license, mode, hash)
	}
	bindings, e := tx.GetBindings(ctx, license)
	if e != nil {
		return domain.Binding{}, e
	}
	for _, b := range bindings {
		if b.Status != domain.BindingActive || b.BindMode != mode {
			continue
		}
		registered, e := yncrypto.DecryptString(s.dataKey, b.BindValueEncrypted)
		if e != nil {
			return b, e
		}
		if matchBinding(mode, registered, value, domain.LegacyTerms(p)) {
			return b, nil
		}
	}
	return domain.Binding{}, sql.ErrNoRows
}
