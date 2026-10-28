package licensing

import (
	"context"
	"encoding/json"
	"strings"
)

type PlanInput struct {
	Name           string `json:"name"`
	Terms          Terms  `json:"terms"`
	PriceCents     int64  `json:"price_cents"`
	AgentCostCents int64  `json:"agent_cost_cents"`
	Currency       string `json:"currency"`
	Enabled        *bool  `json:"enabled"`
}

func (s *Service) ListPlans(ctx context.Context, id string) ([]Plan, error) {
	scoped, _, repo, e := s.applicationContext(ctx, id, false)
	if e != nil {
		return nil, e
	}
	var items []Plan
	e = repo.View(scoped, func(v View) error { var e error; items, e = v.Plans().Find(scoped, Filter{Limit: 1000}); return e })
	return items, e
}
func (s *Service) SavePlan(ctx context.Context, id, planID string, input PlanInput) (Plan, error) {
	scoped, app, repo, e := s.applicationContext(ctx, id, true)
	if e != nil {
		return Plan{}, e
	}
	name := strings.TrimSpace(input.Name)
	if name == "" || len(name) > 160 || input.PriceCents < 0 || input.PriceCents > 1_000_000_000_000 || input.AgentCostCents < 0 || input.AgentCostCents > 1_000_000_000_000 {
		return Plan{}, invalid("套餐名称或金额无效")
	}
	if input.Currency == "" {
		input.Currency = "CNY"
	}
	if input.Currency != "CNY" {
		return Plan{}, invalid("当前结算币种为 CNY")
	}
	terms, e := NormalizeTerms(app, input.Terms)
	if e != nil {
		return Plan{}, e
	}
	var result Plan
	e = repo.Update(scoped, func(tx Transaction) error {
		current, e := tx.Application(scoped)
		if e != nil {
			return e
		}
		terms, e = NormalizeTerms(current, terms)
		if e != nil {
			return e
		}
		for _, target := range terms.RenewalTargetPlanIDs {
			if _, e := tx.Plans().Get(scoped, target); e != nil {
				return invalid("续费目标必须是当前应用已有的套餐")
			}
		}
		if planID == "" {
			result = Plan{Record: s.record(id, "plan"), Name: name, Terms: terms, PriceCents: input.PriceCents, AgentCostCents: input.AgentCostCents, Currency: input.Currency, Enabled: true}
			if input.Enabled != nil {
				result.Enabled = *input.Enabled
			}
			if e = tx.Plans().Insert(scoped, result); e != nil {
				return e
			}
		} else {
			result, e = tx.Plans().Get(scoped, planID)
			if e != nil {
				return notFound()
			}
			old, _ := json.Marshal(result.Terms)
			next, _ := json.Marshal(terms)
			if string(old) != string(next) {
				count, e := tx.Licenses().Count(scoped, Filter{PlanID: planID})
				if e != nil {
					return e
				}
				if count > 0 {
					return conflict("PLAN_TERMS_FROZEN", "已发售套餐的规格已冻结，请复制建立新套餐")
				}
			}
			result.Name = name
			result.Terms = terms
			result.PriceCents = input.PriceCents
			result.AgentCostCents = input.AgentCostCents
			result.Currency = input.Currency
			if input.Enabled != nil {
				result.Enabled = *input.Enabled
			}
			result.UpdatedAt = s.now()
			if e = tx.Plans().Save(scoped, result); e != nil {
				return e
			}
		}
		return s.append(scoped, tx, "plan_saved", result.ID, map[string]any{"name": name, "model": terms.Model, "enabled": result.Enabled})
	})
	return result, e
}
func (s *Service) ListBindingRules(ctx context.Context, id string) ([]BindingRule, error) {
	scoped, _, repo, e := s.applicationContext(ctx, id, false)
	if e != nil {
		return nil, e
	}
	var items []BindingRule
	e = repo.View(scoped, func(v View) error { var e error; items, e = v.Rules().Find(scoped, Filter{Limit: 1000}); return e })
	return items, e
}
func (s *Service) SaveBindingRule(ctx context.Context, id, ruleID string, input BindingRule) (BindingRule, error) {
	scoped, _, repo, e := s.applicationContext(ctx, id, true)
	if e != nil {
		return BindingRule{}, e
	}
	if input.Kind != "domain" && input.Kind != "ip" {
		return BindingRule{}, invalid("白名单类型只能为域名或 IP")
	}
	pattern, e := normalizeRule(input.Kind, input.Pattern)
	if e != nil {
		return BindingRule{}, e
	}
	if input.SubdomainDepth == "" {
		input.SubdomainDepth = "one"
	}
	if input.SubdomainDepth != "one" && input.SubdomainDepth != "all" || len(input.Label) > 160 {
		return BindingRule{}, invalid("规则范围或标签无效")
	}
	var result BindingRule
	e = repo.Update(scoped, func(tx Transaction) error {
		if input.LicenseID != "" {
			if _, e := tx.Licenses().Get(scoped, input.LicenseID); e != nil {
				return notFound()
			}
		}
		if ruleID == "" {
			result = input
			result.Record = s.record(id, "rule")
			result.Pattern = pattern
			if e = tx.Rules().Insert(scoped, result); e != nil {
				return e
			}
		} else {
			var e error
			result, e = tx.Rules().Get(scoped, ruleID)
			if e != nil {
				return notFound()
			}
			result.Kind = input.Kind
			result.Pattern = pattern
			result.Enabled = input.Enabled
			result.IncludeApex = input.IncludeApex
			result.SubdomainDepth = input.SubdomainDepth
			result.LicenseID = input.LicenseID
			result.Label = input.Label
			result.UpdatedAt = s.now()
			if e = tx.Rules().Save(scoped, result); e != nil {
				return e
			}
		}
		return s.append(scoped, tx, "binding_rule_saved", result.ID, map[string]any{"kind": result.Kind, "enabled": result.Enabled, "license_id": result.LicenseID})
	})
	return result, e
}
func (s *Service) RemoveBindingRule(ctx context.Context, id, ruleID, reason string) error {
	scoped, _, repo, e := s.applicationContext(ctx, id, true)
	if e != nil {
		return e
	}
	if strings.TrimSpace(reason) == "" {
		return invalid("请填写删除原因")
	}
	return repo.Update(scoped, func(tx Transaction) error {
		if e := tx.Rules().Delete(scoped, ruleID); e != nil {
			return notFound()
		}
		return s.append(scoped, tx, "binding_rule_removed", ruleID, map[string]any{"reason": reason})
	})
}
func (s *Service) Diagnostics(ctx context.Context, id string) (Diagnostics, error) {
	scoped, _, repo, e := s.applicationContext(ctx, id, false)
	if e != nil {
		return Diagnostics{}, e
	}
	var result Diagnostics
	e = repo.View(scoped, func(v View) error { var e error; result, e = v.Diagnostics(scoped, s.now()); return e })
	return result, e
}
func (s *Service) AuditEvents(ctx context.Context, id string, filter Filter) ([]AuditEvent, error) {
	scoped, _, repo, e := s.applicationContext(ctx, id, false)
	if e != nil {
		return nil, e
	}
	var result []AuditEvent
	e = repo.View(scoped, func(v View) error { var e error; result, e = v.Audit().Find(scoped, filter); return e })
	return result, e
}
