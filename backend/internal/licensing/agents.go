package licensing

import (
	"context"
	"encoding/json"
	"strings"
)

type AgentQuote struct {
	Application   Application `json:"application"`
	Plan          Plan        `json:"plan"`
	Grant         AgentGrant  `json:"grant"`
	UnitCostCents int64       `json:"unit_cost_cents"`
	BalanceCents  int64       `json:"balance_cents"`
	CostRateBPS   int64       `json:"cost_rate_bps"`
}

func (s *Service) SaveAgentGrant(ctx context.Context, appID string, in AgentGrant) (AgentGrant, error) {
	scoped, _, repo, e := s.applicationContext(ctx, appID, true)
	if e != nil {
		return AgentGrant{}, e
	}
	p, _ := PrincipalFrom(ctx)
	if !privileged(p, true) {
		return AgentGrant{}, forbidden("ROLE_FORBIDDEN", "只有平台管理员可配置代理授权与成本")
	}
	if in.AgentID == "" || in.PlanID == "" || in.MaxBatchQuantity < 1 || in.MaxBatchQuantity > 1000 || in.OverrideCostCents != nil && (*in.OverrideCostCents < 0 || *in.OverrideCostCents > 1_000_000_000_000) {
		return AgentGrant{}, invalid("代理、套餐、批量额度或成本无效")
	}
	var out AgentGrant
	e = repo.Update(scoped, func(tx Transaction) error {
		if _, e := tx.AgentAccount(scoped, in.AgentID); e != nil {
			return notFound()
		}
		if _, e := tx.Plans().Get(scoped, in.PlanID); e != nil {
			return notFound()
		}
		old, e := tx.AgentGrants().Find(scoped, Filter{AgentID: in.AgentID, PlanID: in.PlanID, Limit: 1})
		if e != nil {
			return e
		}
		out = in
		if len(old) == 0 {
			out.Record = s.record(appID, "grant")
			e = tx.AgentGrants().Insert(scoped, out)
		} else {
			out.Record = old[0].Record
			out.UpdatedAt = s.now()
			e = tx.AgentGrants().Save(scoped, out)
		}
		if e != nil {
			return e
		}
		return s.append(scoped, tx, "agent_grant_saved", out.ID, map[string]any{"agent_id": in.AgentID, "plan_id": in.PlanID, "enabled": in.Enabled, "override_cost_cents": in.OverrideCostCents})
	})
	return out, e
}
func (s *Service) ListAgentGrants(ctx context.Context, appID string) ([]AgentGrant, error) {
	scoped, _, repo, e := s.applicationContext(ctx, appID, false)
	if e != nil {
		return nil, e
	}
	var out []AgentGrant
	e = repo.View(scoped, func(v View) error { var e error; out, e = v.AgentGrants().Find(scoped, Filter{Limit: 10000}); return e })
	return out, e
}
func quoteAgent(app Application, plan Plan, grant AgentGrant, account AgentAccount) (AgentQuote, error) {
	out := AgentQuote{Application: app, Plan: plan, Grant: grant, BalanceCents: account.BalanceCents, CostRateBPS: account.CostRateBPS}
	if app.Status != "active" || !plan.Enabled || !grant.Enabled || account.Status != "active" {
		return out, forbidden("AGENT_PLAN_NOT_GRANTED", "应用、套餐或代理授权已停用")
	}
	if _, e := NormalizeTerms(app, plan.Terms); e != nil {
		return out, e
	}
	out.UnitCostCents = (plan.AgentCostCents*account.CostRateBPS + 9999) / 10000
	if grant.OverrideCostCents != nil {
		out.UnitCostCents = *grant.OverrideCostCents
	}
	return out, nil
}
func agentPrincipal(ctx context.Context) (Principal, error) {
	p, e := PrincipalFrom(ctx)
	if e != nil {
		return p, e
	}
	if p.Actor.Kind != "agent" || p.AgentID == "" {
		return p, forbidden("ROLE_FORBIDDEN", "需要有效代理身份")
	}
	return p, nil
}
func (s *Service) AgentCatalog(ctx context.Context) ([]AgentQuote, error) {
	p, e := agentPrincipal(ctx)
	if e != nil {
		return nil, e
	}
	apps, e := s.registry.ListApplications(ctx, "")
	if e != nil {
		return nil, e
	}
	out := []AgentQuote{}
	for _, app := range apps {
		scoped, _, repo, e := s.runtimeContext(ctx, app.ApplicationID, p.Actor)
		if e != nil {
			return nil, e
		}
		e = repo.View(scoped, func(v View) error {
			grants, e := v.AgentGrants().Find(scoped, Filter{AgentID: p.AgentID, Limit: 10000})
			if e != nil {
				return e
			}
			if len(grants) == 0 {
				return nil
			}
			account, e := v.AgentAccount(scoped, p.AgentID)
			if e != nil {
				return e
			}
			for _, g := range grants {
				plan, e := v.Plans().Get(scoped, g.PlanID)
				if e != nil {
					return e
				}
				q, e := quoteAgent(app, plan, g, account)
				if e == nil {
					q.Application.SecretHash = ""
					out = append(out, q)
				}
			}
			return nil
		})
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}

type AgentSaleInput struct {
	PlanID                string `json:"plan_id"`
	Quantity              int    `json:"quantity"`
	CustomerRef           string `json:"customer_ref"`
	RequestID             string `json:"request_id"`
	ExpectedUnitCostCents *int64 `json:"expected_unit_cost_cents"`
	Purpose               string `json:"purpose"`
}

func (s *Service) AgentSale(ctx context.Context, appID string, in AgentSaleInput) (SaleResult, error) {
	p, e := agentPrincipal(ctx)
	if e != nil {
		return SaleResult{}, e
	}
	if p.Actor.Role != "agent_owner" && p.Actor.Role != "agent_manager" {
		return SaleResult{}, forbidden("ROLE_FORBIDDEN", "此代理成员没有发卡权限")
	}
	if len(in.RequestID) < 8 || len(in.RequestID) > 128 || in.ExpectedUnitCostCents == nil {
		return SaleResult{}, invalid("请提供业务幂等键与确认成本")
	}
	if e = validateSale(SaleInput{PlanID: in.PlanID, Quantity: in.Quantity, CustomerRef: in.CustomerRef, DistributeCards: true, Purpose: in.Purpose}); e != nil {
		return SaleResult{}, e
	}
	scoped, _, repo, e := s.runtimeContext(ctx, appID, p.Actor)
	if e != nil {
		return SaleResult{}, e
	}
	body, _ := json.Marshal(in)
	fingerprint := s.hash("agent-order", appID, string(body))
	var out SaleResult
	e = repo.Update(scoped, func(tx Transaction) error {
		events, e := tx.MoneyEvents().Find(scoped, Filter{AgentID: p.AgentID, RequestID: in.RequestID, Limit: 1})
		if e != nil {
			return e
		}
		if len(events) > 0 {
			if events[0].RequestHash != fingerprint {
				return conflict("IDEMPOTENCY_CONFLICT", "幂等键已用于不同发卡请求")
			}
			order, e := tx.Orders().Get(scoped, events[0].OrderID)
			out = SaleResult{Order: order, Cards: []CardDelivery{}, Replayed: true}
			return e
		}
		grants, e := tx.AgentGrants().Find(scoped, Filter{AgentID: p.AgentID, PlanID: in.PlanID, Limit: 1})
		if e != nil {
			return e
		}
		if len(grants) != 1 {
			return forbidden("AGENT_PLAN_NOT_GRANTED", "未获此应用套餐的代理权限")
		}
		app, e := tx.Application(scoped)
		if e != nil {
			return e
		}
		plan, e := tx.Plans().Get(scoped, in.PlanID)
		if e != nil {
			return notFound()
		}
		account, e := tx.AgentAccount(scoped, p.AgentID)
		if e != nil {
			return e
		}
		quote, e := quoteAgent(app, plan, grants[0], account)
		if e != nil {
			return e
		}
		if in.Quantity > grants[0].MaxBatchQuantity {
			return forbidden("BATCH_LIMIT_EXCEEDED", "超过本套餐的单批发卡额度")
		}
		if *in.ExpectedUnitCostCents != quote.UnitCostCents {
			return conflict("PRICE_CHANGED", "代理成本已调整，请刷新确认后发卡")
		}
		cost := quote.UnitCostCents * int64(in.Quantity)
		balance, e := tx.DebitAgent(scoped, p.AgentID, cost)
		if e != nil {
			return e
		}
		order := SalesOrder{Record: s.record(appID, "order"), AgentID: p.AgentID, ExternalID: in.RequestID, PlanID: in.PlanID, Quantity: in.Quantity, CustomerRef: strings.TrimSpace(in.CustomerRef), CostCents: cost, Currency: "CNY", Status: "pending", Terms: plan.Terms, DistributeCards: true, Purpose: in.Purpose, Reason: "agent_balance"}
		if e = tx.Orders().Insert(scoped, order); e != nil {
			return e
		}
		out, e = s.issueOrder(scoped, tx, app, order, in.Purpose, nil)
		if e != nil {
			return e
		}
		event := MoneyEvent{Record: s.record(appID, "money"), AgentID: p.AgentID, OrderID: order.ID, RequestID: in.RequestID, RequestHash: fingerprint, AmountCents: -cost, BalanceAfter: balance, Kind: "issue"}
		if e = tx.MoneyEvents().Insert(scoped, event); e != nil {
			return e
		}
		return s.append(scoped, tx, "agent_sale_confirmed", order.ID, map[string]any{"agent_id": p.AgentID, "quantity": in.Quantity, "unit_cost_cents": quote.UnitCostCents, "cost_cents": cost, "balance_after": balance})
	})
	return out, e
}
func (s *Service) AgentOrders(ctx context.Context, appID string) ([]SalesOrder, error) {
	p, e := agentPrincipal(ctx)
	if e != nil {
		return nil, e
	}
	scoped, _, repo, e := s.runtimeContext(ctx, appID, p.Actor)
	if e != nil {
		return nil, e
	}
	var out []SalesOrder
	e = repo.View(scoped, func(v View) error {
		var e error
		out, e = v.Orders().Find(scoped, Filter{AgentID: p.AgentID, Limit: 1000})
		return e
	})
	return out, e
}

type AgentRecords struct {
	Application PublicApplication `json:"application"`
	Orders      []SalesOrder      `json:"orders"`
	Licenses    []License         `json:"licenses"`
	MoneyEvents []MoneyEvent      `json:"money_events"`
}

func (s *Service) AgentHistory(ctx context.Context) ([]AgentRecords, error) {
	p, e := agentPrincipal(ctx)
	if e != nil {
		return nil, e
	}
	return s.agentRecords(ctx, p, p.AgentID)
}
func (s *Service) AdminAgentHistory(ctx context.Context, id string) ([]AgentRecords, error) {
	p, e := PrincipalFrom(ctx)
	if e != nil {
		return nil, e
	}
	if p.Actor.Kind != "admin" || !privileged(p, false) {
		return nil, forbidden("ROLE_FORBIDDEN", "只有平台管理人员可查询代理的全部资金记录")
	}
	if id == "" || len(id) > 96 {
		return nil, invalid("代理编号无效")
	}
	return s.agentRecords(ctx, p, id)
}
func (s *Service) agentRecords(ctx context.Context, p Principal, agentID string) ([]AgentRecords, error) {
	apps, e := s.registry.ListApplications(ctx, "")
	if e != nil {
		return nil, e
	}
	out := []AgentRecords{}
	for _, app := range apps {
		var scoped context.Context
		var repo Repository
		var e error
		if p.Actor.Kind == "admin" {
			scoped, _, repo, e = s.applicationContext(ctx, app.ApplicationID, false)
		} else {
			scoped, _, repo, e = s.runtimeContext(ctx, app.ApplicationID, p.Actor)
		}
		if e != nil {
			return nil, e
		}
		e = repo.View(scoped, func(v View) error {
			orders, e := v.Orders().Find(scoped, Filter{AgentID: agentID, Limit: 10000})
			if e != nil {
				return e
			}
			if len(orders) == 0 {
				return nil
			}
			entry := AgentRecords{Application: PublicApplication{app.ApplicationID, app.Name, app.AllowedBindings}, Orders: orders, Licenses: []License{}}
			for _, order := range orders {
				for _, id := range order.LicenseIDs {
					l, e := v.Licenses().Get(scoped, id)
					if e != nil {
						return e
					}
					if l.OrderID != order.ID {
						return ErrScopeMismatch
					}
					entry.Licenses = append(entry.Licenses, l)
				}
			}
			entry.MoneyEvents, e = v.MoneyEvents().Find(scoped, Filter{AgentID: agentID, Limit: 10000})
			if e != nil {
				return e
			}
			out = append(out, entry)
			return nil
		})
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}

// SaveGroupAgentGrants reports each result; cross-application bulk operations
// deliberately use a fresh authorized scope and transaction for every app.
func (s *Service) SaveGroupAgentGrants(ctx context.Context, group string, grant AgentGrant) (map[string]any, error) {
	p, e := PrincipalFrom(ctx)
	if e != nil {
		return nil, e
	}
	if p.Actor.Kind != "admin" || !privileged(p, true) {
		return nil, forbidden("ROLE_FORBIDDEN", "只有平台管理员可批量授予代理权限")
	}
	group = strings.TrimSpace(group)
	if group == "" {
		return nil, invalid("请选择具体的应用分组")
	}
	apps, e := s.ListApplications(ctx)
	if e != nil {
		return nil, e
	}
	saved := 0
	failures := []map[string]string{}
	for _, app := range apps {
		if app.Group != group {
			continue
		}
		plans, e := s.ListPlans(ctx, app.ApplicationID)
		if e != nil {
			failures = append(failures, map[string]string{"app_id": app.ApplicationID, "code": errorCode(e)})
			continue
		}
		for _, plan := range plans {
			if !plan.Enabled {
				continue
			}
			next := grant
			next.PlanID = plan.ID
			if _, e = s.SaveAgentGrant(ctx, app.ApplicationID, next); e != nil {
				failures = append(failures, map[string]string{"app_id": app.ApplicationID, "plan_id": plan.ID, "code": errorCode(e)})
			} else {
				saved++
			}
		}
	}
	return map[string]any{"saved": saved, "failures": failures}, nil
}
