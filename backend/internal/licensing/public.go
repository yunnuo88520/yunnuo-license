package licensing

import (
	"context"
	"time"
)

type PublicApplication struct {
	AppID    string   `json:"app_id"`
	Name     string   `json:"name"`
	Bindings []string `json:"bindings"`
}

func (s *Service) PublicApplications(ctx context.Context) ([]PublicApplication, error) {
	apps, e := s.registry.ListApplications(ctx, "")
	if e != nil {
		return nil, e
	}
	out := []PublicApplication{}
	for _, app := range apps {
		if app.Status == "active" && app.PublicQueryEnabled {
			out = append(out, PublicApplication{app.ApplicationID, app.Name, app.AllowedBindings})
		}
	}
	return out, nil
}

type PublicLookupInput struct {
	AppID    string       `json:"app_id"`
	Binding  BindingInput `json:"binding"`
	CardCode string       `json:"card_code,omitempty"`
}
type PublicLookupResult struct {
	ServerTime         time.Time         `json:"server_time"`
	Application        PublicApplication `json:"application"`
	Authorized         bool              `json:"authorized"`
	Status             string            `json:"status"`
	Model              string            `json:"model,omitempty"`
	Perpetual          bool              `json:"perpetual"`
	UsageExpiresAt     *time.Time        `json:"usage_expires_at"`
	UpdatesUntil       *time.Time        `json:"updates_until"`
	UpdatesAllowed     bool              `json:"updates_allowed"`
	AwaitingActivation bool              `json:"awaiting_activation"`
}

func (s *Service) PublicLookup(ctx context.Context, in PublicLookupInput) (PublicLookupResult, error) {
	scoped, app, repo, e := s.publicApplicationContext(ctx, in.AppID)
	if e != nil {
		return PublicLookupResult{}, e
	}
	out := PublicLookupResult{Application: PublicApplication{app.ApplicationID, app.Name, app.AllowedBindings}, Status: "not_found", ServerTime: s.now()}
	e = repo.View(scoped, func(v View) error {
		current, e := v.Application(scoped)
		if e != nil {
			return e
		}
		if current.Status != "active" || !current.PublicQueryEnabled {
			return notFound()
		}
		app = current
		ids := []string{}
		var b normalizedBinding
		if in.CardCode != "" {
			cards, e := v.Cards().Find(scoped, Filter{KeyHash: s.cardHash(in.AppID, in.CardCode), Limit: 1})
			if e != nil {
				return e
			}
			if len(cards) == 1 && cards[0].Status != "revoked" && cards[0].Purpose == "activate" {
				ids = append(ids, cards[0].BoundLicenseID)
			}
		} else {
			if in.Binding.Type != "domain" && in.Binding.Type != "ip" {
				return invalid("免登录查询只接收域名、IP 或卡密，其他绑定请登录用户中心查看")
			}
			b, e = s.normalizeBinding(in.AppID, in.Binding, 128)
			if e != nil {
				return e
			}
			anchors, e := v.Anchors().Find(scoped, Filter{Kind: b.Kind, BindingHash: b.Hash, Status: "active", Limit: 10000})
			if e != nil {
				return e
			}
			for _, a := range anchors {
				ids = append(ids, a.LicenseID)
			}
			rules, e := v.Rules().Find(scoped, Filter{Kind: b.Kind, Limit: 10000})
			if e != nil {
				return e
			}
			for _, rule := range rules {
				if rule.LicenseID != "" && ruleMatches(rule, b.Value) {
					ids = append(ids, rule.LicenseID)
				}
			}
		}
		rules, e := v.Rules().Find(scoped, Filter{Limit: 10000})
		if e != nil {
			return e
		}
		seen := map[string]bool{}
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			l, e := v.Licenses().Get(scoped, id)
			if e != nil {
				return e
			}
			if l.RenewalOnly {
				continue
			}
			status := l.Status
			usable := checkUsable(app, l, s.now()) == nil
			if !usable && status == "active" {
				status = "expired"
			}
			if in.CardCode == "" && (s.checkApplicationRules(app, rules, b, l.ID) != nil || !contains(app.AllowedBindings, b.Kind)) {
				usable = false
				status = "policy_denied"
			}
			candidate := PublicLookupResult{Application: out.Application, Authorized: usable, Status: status, Model: l.Terms.Model, Perpetual: l.Terms.Perpetual, UsageExpiresAt: l.ExpiresAt, UpdatesUntil: l.UpdatesUntil, AwaitingActivation: l.ActivatedAt == nil, ServerTime: s.now()}
			candidate.UpdatesAllowed = usable && app.UpdatesEnabled && l.ActivatedAt != nil && l.UpdatesUntil != nil && s.now().Before(*l.UpdatesUntil) && (l.Terms.Perpetual || l.ExpiresAt != nil && s.now().Before(*l.ExpiresAt))
			if out.Status == "not_found" || candidate.Authorized && !out.Authorized {
				out = candidate
			}
			if out.Authorized {
				break
			}
		}
		return nil
	})
	return out, e
}

// PublicAgentApplications returns only publicly listed applications for which
// the platform granted this agent at least one currently sellable plan.
func (s *Service) PublicAgentApplications(ctx context.Context, agentID string) ([]PublicApplication, error) {
	apps, e := s.registry.ListApplications(ctx, "")
	if e != nil {
		return nil, e
	}
	out := []PublicApplication{}
	for _, app := range apps {
		if app.Status != "active" || !app.PublicQueryEnabled {
			continue
		}
		scoped, _, repo, e := s.publicApplicationContext(ctx, app.ApplicationID)
		if e != nil {
			return nil, e
		}
		granted := false
		e = repo.View(scoped, func(v View) error {
			grants, e := v.AgentGrants().Find(scoped, Filter{AgentID: agentID, Limit: 10000})
			if e != nil {
				return e
			}
			for _, g := range grants {
				if !g.Enabled {
					continue
				}
				plan, e := v.Plans().Get(scoped, g.PlanID)
				if e != nil {
					return e
				}
				if _, err := NormalizeTerms(app, plan.Terms); plan.Enabled && err == nil {
					granted = true
					break
				}
			}
			return nil
		})
		if e != nil {
			return nil, e
		}
		if granted {
			out = append(out, PublicApplication{app.ApplicationID, app.Name, app.AllowedBindings})
		}
	}
	return out, nil
}
