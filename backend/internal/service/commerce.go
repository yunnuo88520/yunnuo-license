package service

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

const maxMoney int64 = 1_000_000_000_000

var ErrPackageDenied = forbidden("AGENT_PACKAGE_NOT_ALLOWED", "select an administrator-authorized package")

func (s *Service) SaveProductGroup(ctx context.Context, g domain.ProductGroup, actor string) (domain.ProductGroup, error) {
	g.Name = strings.TrimSpace(g.Name)
	if g.Name == "" || len(g.Name) > 160 || len(g.ProductIDs) > 500 {
		return g, ErrInvalidRequest
	}
	if g.ID == "" {
		g.ID = yncrypto.NewID("grp")
	}
	seen := map[string]bool{}
	for _, id := range g.ProductIDs {
		if seen[id] {
			return g, ErrInvalidRequest
		}
		seen[id] = true
		if _, err := s.currentStore().GetProduct(ctx, id); err != nil {
			return g, ErrProductNotFound
		}
	}
	sort.Strings(g.ProductIDs)
	err := s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.SaveGroup(ctx, g); err != nil {
			return err
		}
		return s.transactionAudit(ctx, tx, "admin", actor, "", "", "group.save", g)
	})
	return g, err
}
func (s *Service) CreatePackage(ctx context.Context, p domain.CardPackage, actor string) (domain.CardPackage, error) {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || len(p.Name) > 160 || p.BaseCost < 0 || p.BaseCost > maxMoney || (!p.IsPermanent && (p.DurationDays < 1 || p.DurationDays > 36500)) {
		return p, ErrInvalidRequest
	}
	if p.IsPermanent {
		p.DurationDays = 0
	}
	p.ID = yncrypto.NewID("pkg")
	p.Enabled = true
	if _, err := s.currentStore().GetProduct(ctx, p.ProductID); err != nil {
		return p, ErrProductNotFound
	}
	err := s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.LockProduct(ctx, p.ProductID); err != nil {
			return err
		}
		if err := tx.CreatePackage(ctx, p); err != nil {
			return err
		}
		return s.transactionAudit(ctx, tx, "admin", actor, p.ProductID, "", "package.create", p)
	})
	return p, err
}
func (s *Service) SetPackage(ctx context.Context, id, actor string, enabled bool, cost int64) error {
	if cost < 0 || cost > maxMoney {
		return ErrInvalidRequest
	}
	p, err := s.currentStore().Package(ctx, id)
	if err != nil {
		return ErrPackageDenied
	}
	return s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.LockProduct(ctx, p.ProductID); err != nil {
			return err
		}
		if err := tx.SetPackage(ctx, id, enabled, cost); err != nil {
			return err
		}
		return s.transactionAudit(ctx, tx, "admin", actor, p.ProductID, "", "package.update", map[string]any{"package_id": id, "enabled": enabled, "base_cost": cost})
	})
}
func (s *Service) CreateTier(ctx context.Context, name, actor string) (domain.AgentTier, error) {
	t := domain.AgentTier{ID: yncrypto.NewID("tier"), Name: strings.TrimSpace(name)}
	if t.Name == "" || len(t.Name) > 160 {
		return t, ErrInvalidRequest
	}
	err := s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.CreateTier(ctx, t); err != nil {
			return err
		}
		return s.transactionAudit(ctx, tx, "admin", actor, "", "", "agent_tier.create", t)
	})
	return t, err
}
func (s *Service) SetTierPrice(ctx context.Context, tier, pkg, actor string, cost *int64) error {
	if cost != nil && (*cost < 0 || *cost > maxMoney) {
		return ErrInvalidRequest
	}
	p, err := s.currentStore().Package(ctx, pkg)
	if err != nil {
		return ErrPackageDenied
	}
	return s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.LockProduct(ctx, p.ProductID); err != nil {
			return err
		}
		if err := tx.SetTierPrice(ctx, tier, pkg, cost); err != nil {
			return err
		}
		return s.transactionAudit(ctx, tx, "admin", actor, p.ProductID, "", "agent_tier.price", map[string]any{"tier_id": tier, "package_id": pkg, "unit_cost": cost})
	})
}
func (s *Service) SetAgentTier(ctx context.Context, agent, tier, actor string) error {
	return s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.LockAgent(ctx, agent); err != nil {
			return err
		}
		if _, err := tx.GetAgent(ctx, agent); err != nil {
			return ErrAgentNotFound
		}
		a, err := tx.Account(ctx, agent)
		if err != nil {
			return err
		}
		a.TierID = tier
		if err := tx.SaveAccount(ctx, a); err != nil {
			return err
		}
		return s.transactionAudit(ctx, tx, "admin", actor, "", "", "agent_tier.assign", a)
	})
}

type GrantPackagesInput struct {
	AgentID          string `json:"-"`
	PackageID        string `json:"package_id"`
	GroupID          string `json:"group_id"`
	Enabled          bool   `json:"enabled"`
	OverrideCost     *int64 `json:"override_cost"`
	MaxBatchQuantity int    `json:"max_batch_quantity"`
	CanExport        bool   `json:"can_export"`
}

// Group grants snapshot the currently enabled packages. Later additions require a new grant.
func (s *Service) GrantPackages(ctx context.Context, in GrantPackagesInput, actor string) error {
	if (in.GroupID == "") == (in.PackageID == "") || in.MaxBatchQuantity < 1 || in.MaxBatchQuantity > 1000 || in.OverrideCost != nil && (*in.OverrideCost < 0 || *in.OverrideCost > maxMoney) {
		return ErrInvalidRequest
	}
	return s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.LockAgent(ctx, in.AgentID); err != nil {
			return err
		}
		if _, err := tx.GetAgent(ctx, in.AgentID); err != nil {
			return ErrAgentNotFound
		}
		var packages []domain.CardPackage
		if in.GroupID != "" {
			var err error
			packages, err = tx.GroupPackages(ctx, in.GroupID)
			if err != nil {
				return err
			}
			if len(packages) == 0 {
				return ErrPackageDenied
			}
		} else {
			p, err := tx.Package(ctx, in.PackageID)
			if err != nil {
				return ErrPackageDenied
			}
			packages = []domain.CardPackage{p}
		}
		for _, p := range packages {
			if err := tx.SaveGrant(ctx, domain.PackageGrant{AgentID: in.AgentID, PackageID: p.ID, Enabled: in.Enabled, OverrideCost: in.OverrideCost}); err != nil {
				return err
			}
			if in.Enabled {
				if err := tx.EnsurePackagePolicy(ctx, in.AgentID, p, in.MaxBatchQuantity, in.CanExport); err != nil {
					return err
				}
			}
		}
		return s.transactionAudit(ctx, tx, "admin", actor, "", "", "agent_packages.grant", map[string]any{"agent_id": in.AgentID, "group_id": in.GroupID, "packages": packages, "enabled": in.Enabled, "override_cost": in.OverrideCost})
	})
}

type AdjustBalanceInput struct {
	Amount    int64  `json:"amount"`
	RequestID string `json:"request_id"`
	Reason    string `json:"reason"`
}

func (s *Service) AdjustAgentBalance(ctx context.Context, agent, actor string, in AdjustBalanceInput) (domain.MoneyLedger, error) {
	var l domain.MoneyLedger
	if in.Amount == 0 || in.Amount > maxMoney || in.Amount < -maxMoney || len(in.RequestID) < 8 || len(in.RequestID) > 64 || strings.TrimSpace(in.Reason) == "" || len(in.Reason) > 500 {
		return l, ErrInvalidRequest
	}
	err := s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.LockAgent(ctx, agent); err != nil {
			return err
		}
		if _, err := tx.GetAgent(ctx, agent); err != nil {
			return ErrAgentNotFound
		}
		old, err := tx.MoneyByRequest(ctx, agent, in.RequestID)
		if err == nil {
			if old.Kind != "adjust" || old.Amount != in.Amount || old.Remark != in.Reason {
				return badRequest("IDEMPOTENCY_CONFLICT", "request ID was already used for different inputs")
			}
			l = old
			return nil
		}
		if !store.IsNotFound(err) {
			return err
		}
		a, err := tx.Account(ctx, agent)
		if err != nil {
			return err
		}
		a.Balance += in.Amount
		if a.Balance < 0 || a.Balance > maxMoney {
			return forbidden("BALANCE_LIMIT", "balance would be negative or exceed the limit")
		}
		if err := tx.SaveAccount(ctx, a); err != nil {
			return err
		}
		l = domain.MoneyLedger{ID: yncrypto.NewID("money"), AgentID: agent, RequestID: in.RequestID, Kind: "adjust", Amount: in.Amount, BalanceAfter: a.Balance, Remark: in.Reason, ActorID: actor, CreatedAt: s.now()}
		if err := tx.InsertMoney(ctx, l); err != nil {
			return err
		}
		return s.transactionAudit(ctx, tx, "admin", actor, "", "", "agent_balance.adjust", l)
	})
	return l, err
}
func (s *Service) AgentPackages(ctx context.Context, agent string) ([]domain.PackageQuote, error) {
	all, err := s.currentStore().Packages(ctx)
	if err != nil {
		return nil, err
	}
	quotes := []domain.PackageQuote{}
	for _, p := range all {
		q, err := s.currentStore().Quote(ctx, agent, p)
		if store.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		policy, err := s.currentStore().GetAgentProductPolicy(ctx, agent, p.ProductID)
		if err != nil {
			if store.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		product, err := s.currentStore().GetProduct(ctx, p.ProductID)
		if err != nil {
			return nil, err
		}
		if product.Status == domain.ProductActive && validateAgentPolicy(policy, 1, p.DurationDays, p.IsPermanent) == nil {
			quotes = append(quotes, q)
		}
	}
	return quotes, nil
}

type CommerceCatalog struct {
	Groups   []domain.ProductGroup `json:"groups"`
	Packages []domain.CardPackage  `json:"packages"`
	Tiers    []domain.AgentTier    `json:"tiers"`
}

func (s *Service) CommerceCatalog(ctx context.Context) (CommerceCatalog, error) {
	var c CommerceCatalog
	var err error
	c.Groups, err = s.currentStore().Groups(ctx)
	if err != nil {
		return c, err
	}
	c.Packages, err = s.currentStore().Packages(ctx)
	if err != nil {
		return c, err
	}
	c.Tiers, err = s.currentStore().Tiers(ctx)
	return c, err
}
func (s *Service) AgentAccount(ctx context.Context, id string) (domain.AgentAccount, error) {
	return s.currentStore().Account(ctx, id)
}
func (s *Service) AgentMoneyLedgers(ctx context.Context, id string) ([]domain.MoneyLedger, error) {
	return s.currentStore().MoneyLedgers(ctx, id)
}

func (s *Service) generatePackageCards(ctx context.Context, in AgentCreateCardBatchInput) (CreateCardBatchResult, error) {
	var result CreateCardBatchResult
	if in.AgentID == "" || in.Quantity < 1 || in.Quantity > 1000 || len(in.Name) > 160 {
		return result, ErrInvalidRequest
	}
	// Retain duration/product fields only as consistency assertions for older integrations.
	if in.PackageID == "" {
		return result, ErrPackageDenied
	}
	if len(in.RequestID) < 8 || len(in.RequestID) > 64 || in.ExpectedUnitCost == nil {
		return result, badRequest("PACKAGE_QUOTE_REQUIRED", "request_id and expected_unit_cost are required")
	}
	pkg, err := s.currentStore().Package(ctx, in.PackageID)
	if err != nil {
		return result, ErrPackageDenied
	}
	product, err := s.currentStore().GetProduct(ctx, pkg.ProductID)
	if err != nil {
		return result, ErrProductNotFound
	}
	if in.ProductID != "" && in.ProductID != pkg.ProductID || in.DurationDays != 0 && in.DurationDays != pkg.DurationDays || in.IsPermanent && !pkg.IsPermanent {
		return result, badRequest("PACKAGE_TERMS_MISMATCH", "package terms cannot be overridden")
	}
	fingerprintBytes, _ := json.Marshal(struct {
		Package, Name string
		Quantity      int
		Cost          int64
	}{in.PackageID, in.Name, in.Quantity, *in.ExpectedUnitCost})
	fingerprint := string(fingerprintBytes)
	replayBatch := ""
	err = s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		// Lock before any transactional read so MySQL's snapshot sees the previous commit.
		if err := tx.LockAgent(ctx, in.AgentID); err != nil {
			return err
		}
		if err := tx.LockProduct(ctx, pkg.ProductID); err != nil {
			return err
		}
		agent, err := tx.GetAgent(ctx, in.AgentID)
		if err != nil {
			return ErrAgentNotFound
		}
		if err := ensureAgentActive(agent); err != nil {
			return err
		}
		old, existingBatch, err := tx.PackageOrder(ctx, in.AgentID, in.RequestID)
		if err == nil {
			if old != fingerprint {
				return badRequest("IDEMPOTENCY_CONFLICT", "request ID was already used for different inputs")
			}
			replayBatch = existingBatch
			return nil
		}
		if !store.IsNotFound(err) {
			return err
		}
		fresh, err := tx.GetProduct(ctx, pkg.ProductID)
		if err != nil {
			return err
		}
		product = fresh
		if product.Status != domain.ProductActive {
			return ErrProductDisabled
		}
		pkg, err = tx.Package(ctx, in.PackageID)
		if err != nil {
			return ErrPackageDenied
		}
		quote, err := tx.Quote(ctx, in.AgentID, pkg)
		if store.IsNotFound(err) {
			return ErrPackageDenied
		}
		if err != nil {
			return err
		}
		if *in.ExpectedUnitCost != quote.UnitCost {
			return badRequest("PACKAGE_PRICE_CHANGED", "package price changed; refresh before submitting")
		}
		policy, err := tx.GetAgentProductPolicy(ctx, in.AgentID, pkg.ProductID)
		if store.IsNotFound(err) {
			return ErrAgentProductDenied
		}
		if err != nil {
			return err
		}
		if err := validateAgentPolicy(policy, in.Quantity, pkg.DurationDays, pkg.IsPermanent); err != nil {
			return err
		}
		a, err := tx.Account(ctx, in.AgentID)
		if err != nil {
			return err
		}
		cost := quote.UnitCost * int64(in.Quantity)
		if cost < 0 || cost > maxMoney || a.Balance < cost {
			return forbidden("AGENT_BALANCE_NOT_ENOUGH", "agent balance is insufficient")
		}
		balance := 0
		if pkg.UseQuota {
			balance, err = tx.AgentQuotaBalance(ctx, in.AgentID, pkg.ProductID, pkg.DurationDays, pkg.IsPermanent)
			if err != nil {
				return err
			}
			if balance < in.Quantity {
				return ErrAgentQuotaNotEnough
			}
		}
		batch, cards, codes, err := s.buildCardBatch(product, in.AgentID, nonEmpty(in.Name, pkg.Name), in.Quantity, pkg.DurationDays, pkg.IsPermanent, "agent_package")
		if err != nil {
			return err
		}
		if err := tx.CreateCardBatch(ctx, batch); err != nil {
			return err
		}
		for _, c := range cards {
			if err := tx.CreateCard(ctx, c); err != nil {
				return err
			}
		}
		a.Balance -= cost
		if err := tx.SaveAccount(ctx, a); err != nil {
			return err
		}
		if pkg.UseQuota {
			if err := tx.InsertAgentQuotaLedger(ctx, domain.AgentQuotaLedger{ID: yncrypto.NewID("quota"), AgentID: in.AgentID, ProductID: pkg.ProductID, DurationDays: pkg.DurationDays, IsPermanent: pkg.IsPermanent, ChangeType: domain.QuotaGenerateCards, ChangeQuantity: -in.Quantity, BalanceAfter: balance - in.Quantity, RelatedBatchID: batch.ID, OperatorType: "agent", OperatorID: in.AgentID, CreatedAt: s.now()}); err != nil {
				return err
			}
		}
		actor := in.AgentID
		if meta := ActorFrom(ctx); meta != nil && meta.ID != "" {
			actor = meta.ID
		}
		l := domain.MoneyLedger{ID: yncrypto.NewID("money"), AgentID: in.AgentID, RequestID: in.RequestID, Kind: "issue", Amount: -cost, BalanceAfter: a.Balance, BatchID: batch.ID, PackageID: pkg.ID, UnitCost: quote.UnitCost, Quantity: in.Quantity, Remark: pkg.Name, ActorID: actor, CreatedAt: s.now()}
		if err := tx.InsertMoney(ctx, l); err != nil {
			return err
		}
		if err := tx.InsertPackageOrder(ctx, in.AgentID, in.RequestID, fingerprint, batch.ID); err != nil {
			return err
		}
		if err := s.transactionAudit(ctx, tx, "agent", actor, pkg.ProductID, "", "agent_package.issue", l); err != nil {
			return err
		}
		result = CreateCardBatchResult{Batch: batch, Codes: codes}
		return nil
	})
	if err != nil {
		return CreateCardBatchResult{}, err
	}
	if replayBatch != "" {
		batch, err := s.currentStore().GetCardBatchForAgent(ctx, replayBatch, in.AgentID)
		if err != nil {
			return result, err
		}
		cards, err := s.currentStore().ListCardsByBatchForAgent(ctx, replayBatch, in.AgentID)
		if err != nil {
			return result, err
		}
		result = CreateCardBatchResult{Batch: batch, Codes: []string{}}
		for _, c := range cards {
			code, err := yncrypto.DecryptString(s.dataKey, c.CodeEncrypted)
			if err != nil {
				return result, err
			}
			result.Codes = append(result.Codes, yncrypto.FormatCardCode(c.CodePrefix, strings.TrimPrefix(code, strings.ToUpper(c.CodePrefix))))
		}
	}
	return result, nil
}
