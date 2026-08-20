package service

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

type Service struct {
	store      atomic.Pointer[store.Store]
	setupMu    sync.Mutex
	retiredMu  sync.Mutex
	retired    []*store.Store
	cardPepper []byte
	dataKey    []byte
	now        func() time.Time
}

func (s *Service) currentStore() *store.Store {
	return s.store.Load()
}

func (s *Service) replaceStore(st *store.Store) {
	old := s.store.Swap(st)
	if old == nil || old == st {
		return
	}
	s.retiredMu.Lock()
	s.retired = append(s.retired, old)
	s.retiredMu.Unlock()
}

// Close is called after the HTTP server has stopped, so no request can still
// be using a store retained during first-run initialization.
func (s *Service) Close() error {
	current := s.store.Swap(nil)
	s.retiredMu.Lock()
	stores := append(s.retired, current)
	s.retired = nil
	s.retiredMu.Unlock()
	var closeErr error
	for _, st := range stores {
		if st != nil {
			closeErr = errors.Join(closeErr, st.Close())
		}
	}
	return closeErr
}

func New(st *store.Store, cardPepper, dataKey []byte) *Service {
	s := &Service{
		cardPepper: cardPepper,
		dataKey:    dataKey,
		now:        func() time.Time { return time.Now().UTC() },
	}
	s.store.Store(st)
	return s
}

type CreateProductInput struct {
	Name                 string `json:"name"`
	Code                 string `json:"code"`
	BindMode             string `json:"bind_mode"`
	MaxBindCount         int    `json:"max_bind_count"`
	BindConflictStrategy string `json:"bind_conflict_strategy"`
	OfflineMode          string `json:"offline_mode"`
	OfflineGraceDays     int    `json:"offline_grace_days"`
	ExpireGraceDays      int    `json:"expire_grace_days"`
	UnbindLimit          int    `json:"unbind_limit"`
	UnbindCooldownHours  int    `json:"unbind_cooldown_hours"`
}

func (s *Service) CreateProduct(ctx context.Context, input CreateProductInput) (domain.Product, error) {
	if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Code) == "" {
		return domain.Product{}, ErrInvalidRequest
	}
	if input.BindMode == "" {
		input.BindMode = domain.BindDevice
	}
	if input.MaxBindCount == 0 {
		input.MaxBindCount = 1
	}
	if input.BindConflictStrategy == "" {
		input.BindConflictStrategy = domain.ConflictReject
	}
	if input.OfflineMode == "" {
		input.OfflineMode = "online_cache"
	}
	if input.OfflineGraceDays == 0 {
		input.OfflineGraceDays = 15
	}
	publicPEM, privatePEM, err := yncrypto.GenerateRSAKeyPair()
	if err != nil {
		return domain.Product{}, err
	}
	privateEncrypted, err := yncrypto.EncryptString(s.dataKey, privatePEM)
	if err != nil {
		return domain.Product{}, err
	}
	now := s.now()
	product := domain.Product{
		ID:                   yncrypto.NewID("prod"),
		Name:                 strings.TrimSpace(input.Name),
		Code:                 strings.ToUpper(strings.TrimSpace(input.Code)),
		AppKey:               yncrypto.NewID("app"),
		PublicKeyPEM:         publicPEM,
		PrivateKeyEncrypted:  privateEncrypted,
		KeyVersion:           1,
		BindMode:             input.BindMode,
		MaxBindCount:         input.MaxBindCount,
		BindConflictStrategy: input.BindConflictStrategy,
		OfflineMode:          input.OfflineMode,
		OfflineGraceDays:     input.OfflineGraceDays,
		ExpireGraceDays:      input.ExpireGraceDays,
		UnbindLimit:          input.UnbindLimit,
		UnbindCooldownHours:  input.UnbindCooldownHours,
		Status:               domain.ProductActive,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := s.currentStore().CreateProduct(ctx, product); err != nil {
		return domain.Product{}, err
	}
	_ = s.audit(ctx, "admin", "", product.ID, "", "", "product.create", "success", "")
	return product, nil
}

func (s *Service) ListProducts(ctx context.Context) ([]domain.Product, error) {
	return s.currentStore().ListProducts(ctx)
}

type ProductKeyRing struct {
	ProductID      string              `json:"product_id"`
	ProductCode    string              `json:"product_code"`
	AppKey         string              `json:"app_key"`
	CurrentVersion int                 `json:"current_version"`
	Keys           []domain.ProductKey `json:"keys"`
}

func (s *Service) ProductKeys(ctx context.Context, productID string) (ProductKeyRing, error) {
	product, err := s.currentStore().GetProduct(ctx, strings.TrimSpace(productID))
	if err != nil {
		if store.IsNotFound(err) {
			return ProductKeyRing{}, ErrProductNotFound
		}
		return ProductKeyRing{}, err
	}
	keys, err := s.currentStore().ListProductKeys(ctx, product.ID)
	if err != nil {
		return ProductKeyRing{}, err
	}
	return ProductKeyRing{ProductID: product.ID, ProductCode: product.Code, AppKey: product.AppKey, CurrentVersion: product.KeyVersion, Keys: keys}, nil
}

func (s *Service) ProductKeysByAppKey(ctx context.Context, appKey string) (ProductKeyRing, error) {
	product, err := s.currentStore().GetProductByAppKey(ctx, strings.TrimSpace(appKey))
	if err != nil {
		if store.IsNotFound(err) {
			return ProductKeyRing{}, ErrProductNotFound
		}
		return ProductKeyRing{}, err
	}
	ring, err := s.ProductKeys(ctx, product.ID)
	if err != nil {
		return ProductKeyRing{}, err
	}
	for i := range ring.Keys {
		ring.Keys[i].CreatedBy = ""
	}
	return ring, nil
}

func (s *Service) RotateProductKey(ctx context.Context, productID, actorID string) (ProductKeyRing, error) {
	product, err := s.currentStore().GetProduct(ctx, strings.TrimSpace(productID))
	if err != nil {
		if store.IsNotFound(err) {
			return ProductKeyRing{}, ErrProductNotFound
		}
		return ProductKeyRing{}, err
	}
	publicPEM, privatePEM, err := yncrypto.GenerateRSAKeyPair()
	if err != nil {
		return ProductKeyRing{}, err
	}
	privateEncrypted, err := yncrypto.EncryptString(s.dataKey, privatePEM)
	if err != nil {
		return ProductKeyRing{}, err
	}
	version, err := s.currentStore().RotateProductKey(ctx, product.ID, publicPEM, privateEncrypted, actorID, s.now())
	if err != nil {
		return ProductKeyRing{}, err
	}
	_ = s.audit(ctx, "admin", actorID, product.ID, "", "", "product_key.rotate", "success", "")
	ring, err := s.ProductKeys(ctx, product.ID)
	if err != nil {
		return ProductKeyRing{}, err
	}
	ring.CurrentVersion = version
	return ring, nil
}

type CreateCardBatchInput struct {
	ProductID    string `json:"product_id"`
	Name         string `json:"name"`
	Quantity     int    `json:"quantity"`
	DurationDays int    `json:"duration_days"`
	IsPermanent  bool   `json:"is_permanent"`
}

type CreateCardBatchResult struct {
	Batch domain.CardBatch `json:"batch"`
	Codes []string         `json:"codes"`
}

func (s *Service) CreateCardBatch(ctx context.Context, input CreateCardBatchInput) (CreateCardBatchResult, error) {
	if input.ProductID == "" || input.Quantity <= 0 || input.Quantity > 1000 {
		return CreateCardBatchResult{}, ErrInvalidRequest
	}
	if !input.IsPermanent && input.DurationDays <= 0 {
		return CreateCardBatchResult{}, ErrInvalidRequest
	}
	product, err := s.currentStore().GetProduct(ctx, input.ProductID)
	if err != nil {
		if store.IsNotFound(err) {
			return CreateCardBatchResult{}, ErrInvalidRequest
		}
		return CreateCardBatchResult{}, err
	}
	if product.Status != domain.ProductActive {
		return CreateCardBatchResult{}, ErrProductDisabled
	}
	batch, cards, codes, err := s.buildCardBatch(product, "", nonEmpty(input.Name, "Manual batch"), input.Quantity, input.DurationDays, input.IsPermanent, "platform_generated")
	if err != nil {
		return CreateCardBatchResult{}, err
	}
	if err := s.currentStore().CreateCardBatch(ctx, batch, cards); err != nil {
		return CreateCardBatchResult{}, err
	}
	_ = s.audit(ctx, "admin", "", product.ID, "", "", "card_batch.create", "success", "")
	return CreateCardBatchResult{Batch: batch, Codes: codes}, nil
}

func (s *Service) ListCardBatches(ctx context.Context) ([]domain.CardBatch, error) {
	return s.currentStore().ListCardBatches(ctx)
}

func (s *Service) ListCardsByBatch(ctx context.Context, batchID string) ([]domain.Card, error) {
	return s.currentStore().ListCardsByBatch(ctx, batchID)
}

type CreateAgentInput struct {
	Name        string `json:"name"`
	ContactName string `json:"contact_name"`
	Phone       string `json:"phone"`
	Email       string `json:"email"`
	Remark      string `json:"remark"`
}

func (s *Service) CreateAgent(ctx context.Context, input CreateAgentInput) (domain.Agent, error) {
	if strings.TrimSpace(input.Name) == "" {
		return domain.Agent{}, ErrInvalidRequest
	}
	loginCode, err := s.newAgentLoginCode(ctx)
	if err != nil {
		return domain.Agent{}, err
	}
	now := s.now()
	agent := domain.Agent{
		ID:                  yncrypto.NewID("agent"),
		AgentNo:             strings.ToUpper(yncrypto.NewID("agt")),
		LoginCode:           loginCode,
		Name:                strings.TrimSpace(input.Name),
		ContactName:         strings.TrimSpace(input.ContactName),
		Phone:               strings.TrimSpace(input.Phone),
		Email:               strings.TrimSpace(input.Email),
		Level:               1,
		Status:              domain.AgentActive,
		SettlementMode:      domain.SettlementManual,
		DefaultDiscountRate: 1,
		Remark:              strings.TrimSpace(input.Remark),
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := s.currentStore().CreateAgentWithLoginCode(ctx, agent); err != nil {
		return domain.Agent{}, err
	}
	_ = s.audit(ctx, "admin", "", "", "", "", "agent.create", "success", "")
	return agent, nil
}

func (s *Service) ListAgents(ctx context.Context) ([]domain.Agent, error) {
	return s.currentStore().ListAgents(ctx)
}

func (s *Service) EnsureAgentLoginCodes(ctx context.Context) error {
	agents, err := s.currentStore().ListAgents(ctx)
	if err != nil {
		return err
	}
	for _, agent := range agents {
		if agent.LoginCode != "" {
			continue
		}
		loginCode, err := s.newAgentLoginCode(ctx)
		if err != nil {
			return err
		}
		if err := s.currentStore().CreateAgentLoginCode(ctx, agent.ID, loginCode, s.now()); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) newAgentLoginCode(ctx context.Context) (string, error) {
	for attempt := 0; attempt < 10; attempt++ {
		loginCode, err := yncrypto.GenerateAgentLoginCode()
		if err != nil {
			return "", err
		}
		if _, err := s.currentStore().GetAgentByLoginCode(ctx, loginCode); store.IsNotFound(err) {
			return loginCode, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", errors.New("could not generate unique agent login code")
}

type AgentProductPolicyInput struct {
	AgentID             string `json:"agent_id"`
	ProductID           string `json:"product_id"`
	CanGenerate         bool   `json:"can_generate"`
	CanExportPlainCode  bool   `json:"can_export_plain_code"`
	AllowedDurationDays []int  `json:"allowed_duration_days"`
	AllowPermanent      bool   `json:"allow_permanent"`
	MaxBatchQuantity    int    `json:"max_batch_quantity"`
}

func (s *Service) UpsertAgentProductPolicy(ctx context.Context, input AgentProductPolicyInput) (domain.AgentProductPolicy, error) {
	if input.AgentID == "" || input.ProductID == "" {
		return domain.AgentProductPolicy{}, ErrInvalidRequest
	}
	if _, err := s.currentStore().GetAgent(ctx, input.AgentID); err != nil {
		if store.IsNotFound(err) {
			return domain.AgentProductPolicy{}, ErrAgentNotFound
		}
		return domain.AgentProductPolicy{}, err
	}
	if _, err := s.currentStore().GetProduct(ctx, input.ProductID); err != nil {
		if store.IsNotFound(err) {
			return domain.AgentProductPolicy{}, ErrInvalidRequest
		}
		return domain.AgentProductPolicy{}, err
	}
	if input.MaxBatchQuantity <= 0 {
		input.MaxBatchQuantity = 100
	}
	now := s.now()
	policy := domain.AgentProductPolicy{
		ID:                  yncrypto.NewID("apol"),
		AgentID:             input.AgentID,
		ProductID:           input.ProductID,
		CanGenerate:         input.CanGenerate,
		CanExportPlainCode:  input.CanExportPlainCode,
		AllowedDurationDays: input.AllowedDurationDays,
		AllowPermanent:      input.AllowPermanent,
		MaxBatchQuantity:    input.MaxBatchQuantity,
		DiscountRate:        1,
		SettlementPrice:     0,
		Status:              domain.AgentPolicyActive,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := s.currentStore().UpsertAgentProductPolicy(ctx, policy); err != nil {
		return domain.AgentProductPolicy{}, err
	}
	_ = s.audit(ctx, "admin", "", input.ProductID, "", "", "agent_policy.upsert", "success", "")
	return policy, nil
}

func (s *Service) ListAgentProductPolicies(ctx context.Context, agentID string) ([]domain.AgentProductPolicy, error) {
	if agentID == "" {
		return nil, ErrInvalidRequest
	}
	return s.currentStore().ListAgentProductPolicies(ctx, agentID)
}

type AgentQuotaInput struct {
	AgentID      string `json:"agent_id"`
	ProductID    string `json:"product_id"`
	DurationDays int    `json:"duration_days"`
	IsPermanent  bool   `json:"is_permanent"`
	Quantity     int    `json:"quantity"`
	Remark       string `json:"remark"`
}

func (s *Service) GrantAgentQuota(ctx context.Context, input AgentQuotaInput) (domain.AgentQuotaLedger, error) {
	if input.AgentID == "" || input.ProductID == "" || input.Quantity <= 0 {
		return domain.AgentQuotaLedger{}, ErrInvalidRequest
	}
	if !input.IsPermanent && input.DurationDays <= 0 {
		return domain.AgentQuotaLedger{}, ErrInvalidRequest
	}
	now := s.now()
	var ledger domain.AgentQuotaLedger
	err := s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		agent, err := tx.GetAgent(ctx, input.AgentID)
		if err != nil {
			if store.IsNotFound(err) {
				return ErrAgentNotFound
			}
			return err
		}
		if err := ensureAgentActive(agent); err != nil {
			return err
		}
		balance, err := tx.AgentQuotaBalance(ctx, input.AgentID, input.ProductID, input.DurationDays, input.IsPermanent)
		if err != nil {
			return err
		}
		ledger = domain.AgentQuotaLedger{
			ID:             yncrypto.NewID("quota"),
			AgentID:        input.AgentID,
			ProductID:      input.ProductID,
			DurationDays:   input.DurationDays,
			IsPermanent:    input.IsPermanent,
			ChangeType:     domain.QuotaGrant,
			ChangeQuantity: input.Quantity,
			BalanceAfter:   balance + input.Quantity,
			OperatorType:   "admin",
			Remark:         input.Remark,
			CreatedAt:      now,
		}
		return tx.InsertAgentQuotaLedger(ctx, ledger)
	})
	if err != nil {
		return domain.AgentQuotaLedger{}, err
	}
	_ = s.audit(ctx, "admin", "", input.ProductID, "", "", "agent_quota.grant", "success", "")
	return ledger, nil
}

func (s *Service) ListAgentQuotaLedgers(ctx context.Context, agentID string) ([]domain.AgentQuotaLedger, error) {
	if agentID == "" {
		return nil, ErrInvalidRequest
	}
	return s.currentStore().ListAgentQuotaLedgers(ctx, agentID)
}

func (s *Service) ListAgentQuotaSummaries(ctx context.Context, agentID string) ([]domain.AgentQuotaSummary, error) {
	if agentID == "" {
		return nil, ErrInvalidRequest
	}
	return s.currentStore().ListAgentQuotaSummaries(ctx, agentID)
}

type AgentProductView struct {
	ID     string                    `json:"id"`
	Name   string                    `json:"name"`
	Code   string                    `json:"code"`
	Status string                    `json:"status"`
	Policy domain.AgentProductPolicy `json:"policy"`
}

func (s *Service) ListAgentProducts(ctx context.Context, agentID string) ([]AgentProductView, error) {
	if strings.TrimSpace(agentID) == "" {
		return nil, ErrInvalidRequest
	}
	policies, err := s.currentStore().ListAgentProductPolicies(ctx, agentID)
	if err != nil {
		return nil, err
	}
	products := make([]AgentProductView, 0, len(policies))
	for _, policy := range policies {
		product, err := s.currentStore().GetProduct(ctx, policy.ProductID)
		if err != nil {
			if store.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		products = append(products, AgentProductView{
			ID:     product.ID,
			Name:   product.Name,
			Code:   product.Code,
			Status: product.Status,
			Policy: policy,
		})
	}
	return products, nil
}

type AgentCreateCardBatchInput struct {
	AgentID      string `json:"-"`
	ProductID    string `json:"product_id"`
	Name         string `json:"name"`
	Quantity     int    `json:"quantity"`
	DurationDays int    `json:"duration_days"`
	IsPermanent  bool   `json:"is_permanent"`
}

func (s *Service) AgentCreateCardBatch(ctx context.Context, input AgentCreateCardBatchInput) (CreateCardBatchResult, error) {
	if input.AgentID == "" || input.ProductID == "" || input.Quantity <= 0 {
		return CreateCardBatchResult{}, ErrInvalidRequest
	}
	if !input.IsPermanent && input.DurationDays <= 0 {
		return CreateCardBatchResult{}, ErrInvalidRequest
	}
	product, err := s.currentStore().GetProduct(ctx, input.ProductID)
	if err != nil {
		if store.IsNotFound(err) {
			return CreateCardBatchResult{}, ErrInvalidRequest
		}
		return CreateCardBatchResult{}, err
	}
	if product.Status != domain.ProductActive {
		return CreateCardBatchResult{}, ErrProductDisabled
	}
	batch, cards, codes, err := s.buildCardBatch(product, input.AgentID, nonEmpty(input.Name, "Agent batch"), input.Quantity, input.DurationDays, input.IsPermanent, "agent_self_generated")
	if err != nil {
		return CreateCardBatchResult{}, err
	}
	now := s.now()
	err = s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		agent, err := tx.GetAgent(ctx, input.AgentID)
		if err != nil {
			if store.IsNotFound(err) {
				return ErrAgentNotFound
			}
			return err
		}
		if err := ensureAgentActive(agent); err != nil {
			return err
		}
		policy, err := tx.GetAgentProductPolicy(ctx, input.AgentID, input.ProductID)
		if err != nil {
			if store.IsNotFound(err) {
				return ErrAgentProductDenied
			}
			return err
		}
		if err := validateAgentPolicy(policy, input.Quantity, input.DurationDays, input.IsPermanent); err != nil {
			return err
		}
		balance, err := tx.AgentQuotaBalance(ctx, input.AgentID, input.ProductID, input.DurationDays, input.IsPermanent)
		if err != nil {
			return err
		}
		if balance < input.Quantity {
			return ErrAgentQuotaNotEnough
		}
		if err := tx.CreateCardBatch(ctx, batch); err != nil {
			return err
		}
		for _, card := range cards {
			if err := tx.CreateCard(ctx, card); err != nil {
				return err
			}
		}
		ledger := domain.AgentQuotaLedger{
			ID:             yncrypto.NewID("quota"),
			AgentID:        input.AgentID,
			ProductID:      input.ProductID,
			DurationDays:   input.DurationDays,
			IsPermanent:    input.IsPermanent,
			ChangeType:     domain.QuotaGenerateCards,
			ChangeQuantity: -input.Quantity,
			BalanceAfter:   balance - input.Quantity,
			RelatedBatchID: batch.ID,
			OperatorType:   "agent",
			OperatorID:     input.AgentID,
			Remark:         "agent self generated cards",
			CreatedAt:      now,
		}
		return tx.InsertAgentQuotaLedger(ctx, ledger)
	})
	if err != nil {
		_ = s.auditAgent(ctx, "", input.AgentID, input.ProductID, "agent_card_batch.create", "failed", errorCode(err))
		return CreateCardBatchResult{}, err
	}
	_ = s.auditAgent(ctx, "", input.AgentID, input.ProductID, "agent_card_batch.create", "success", "")
	return CreateCardBatchResult{Batch: batch, Codes: codes}, nil
}

func (s *Service) ListAgentCardBatches(ctx context.Context, agentID string) ([]domain.CardBatch, error) {
	if strings.TrimSpace(agentID) == "" {
		return nil, ErrInvalidRequest
	}
	return s.currentStore().ListCardBatchesByAgent(ctx, agentID)
}

func (s *Service) ListAgentCardsByBatch(ctx context.Context, agentID, batchID string) ([]domain.Card, error) {
	if strings.TrimSpace(agentID) == "" || strings.TrimSpace(batchID) == "" {
		return nil, ErrInvalidRequest
	}
	if _, err := s.currentStore().GetCardBatchForAgent(ctx, batchID, agentID); err != nil {
		if store.IsNotFound(err) {
			return nil, ErrAgentBatchNotFound
		}
		return nil, err
	}
	return s.currentStore().ListCardsByBatchForAgent(ctx, batchID, agentID)
}

type AgentCardExportResult struct {
	Batch domain.CardBatch `json:"batch"`
	Codes []string         `json:"codes"`
}

func (s *Service) ExportAgentCardBatch(ctx context.Context, agentID, userID, batchID string) (AgentCardExportResult, error) {
	if strings.TrimSpace(agentID) == "" || strings.TrimSpace(batchID) == "" {
		return AgentCardExportResult{}, ErrInvalidRequest
	}
	batch, err := s.currentStore().GetCardBatchForAgent(ctx, batchID, agentID)
	if err != nil {
		if store.IsNotFound(err) {
			return AgentCardExportResult{}, ErrAgentBatchNotFound
		}
		return AgentCardExportResult{}, err
	}
	policy, err := s.currentStore().GetAgentProductPolicy(ctx, agentID, batch.ProductID)
	if err != nil {
		if store.IsNotFound(err) {
			return AgentCardExportResult{}, ErrAgentExportDenied
		}
		return AgentCardExportResult{}, err
	}
	if policy.Status != domain.AgentPolicyActive || !policy.CanExportPlainCode {
		return AgentCardExportResult{}, ErrAgentExportDenied
	}
	cards, err := s.currentStore().ListCardsByBatchForAgent(ctx, batchID, agentID)
	if err != nil {
		return AgentCardExportResult{}, err
	}
	codes := make([]string, 0, len(cards))
	for _, card := range cards {
		code, err := yncrypto.DecryptString(s.dataKey, card.CodeEncrypted)
		if err != nil {
			return AgentCardExportResult{}, err
		}
		codes = append(codes, code)
	}
	now := s.now()
	if err := s.currentStore().IncrementCardBatchExportCount(ctx, batchID, now); err != nil {
		return AgentCardExportResult{}, err
	}
	batch.ExportCount++
	batch.UpdatedAt = now
	_ = s.auditAgent(ctx, userID, agentID, batch.ProductID, "agent_card_batch.export", "success", "")
	return AgentCardExportResult{Batch: batch, Codes: codes}, nil
}

func (s *Service) ListAgentLicenses(ctx context.Context, agentID string) ([]domain.License, error) {
	if strings.TrimSpace(agentID) == "" {
		return nil, ErrInvalidRequest
	}
	return s.currentStore().ListLicensesByAgent(ctx, agentID)
}

func (s *Service) ListLicenses(ctx context.Context) ([]domain.License, error) {
	return s.currentStore().ListLicenses(ctx)
}

func (s *Service) ListLicenseBindings(ctx context.Context, licenseNo string) ([]domain.Binding, error) {
	lic, err := s.currentStore().GetLicenseByNo(ctx, licenseNo)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, ErrLicenseNotFound
		}
		return nil, err
	}
	return s.currentStore().GetBindings(ctx, lic.ID)
}

type PublicLicenseQueryInput struct {
	LicenseNo string `json:"license_no"`
	CardCode  string `json:"card_code"`
	Query     string `json:"query"`
	QueryType string `json:"query_type"`
}

type PublicLicenseQueryMatch struct {
	ProductName   string     `json:"product_name"`
	ProductCode   string     `json:"product_code"`
	LicenseNo     string     `json:"license_no"`
	LicenseStatus string     `json:"license_status"`
	IsPermanent   bool       `json:"is_permanent"`
	ActivatedAt   *time.Time `json:"activated_at,omitempty"`
	ExpiredAt     *time.Time `json:"expired_at,omitempty"`
	LastVerifyAt  *time.Time `json:"last_verify_at,omitempty"`
}

type PublicLicenseQueryResult struct {
	QueryType     string                    `json:"query_type"`
	QueryMasked   string                    `json:"query_masked,omitempty"`
	MatchCount    int                       `json:"match_count,omitempty"`
	Results       []PublicLicenseQueryMatch `json:"results,omitempty"`
	ProductName   string                    `json:"product_name"`
	ProductCode   string                    `json:"product_code"`
	CardStatus    string                    `json:"card_status,omitempty"`
	LicenseNo     string                    `json:"license_no,omitempty"`
	LicenseStatus string                    `json:"license_status,omitempty"`
	DurationDays  int                       `json:"duration_days"`
	IsPermanent   bool                      `json:"is_permanent"`
	ActivatedAt   *time.Time                `json:"activated_at,omitempty"`
	ExpiredAt     *time.Time                `json:"expired_at,omitempty"`
	LastVerifyAt  *time.Time                `json:"last_verify_at,omitempty"`
	ServerTime    time.Time                 `json:"server_time"`
}

func (s *Service) QueryPublicLicense(ctx context.Context, input PublicLicenseQueryInput) (PublicLicenseQueryResult, error) {
	input.LicenseNo = strings.TrimSpace(input.LicenseNo)
	input.CardCode = strings.TrimSpace(input.CardCode)
	input.Query = strings.TrimSpace(input.Query)
	input.QueryType = strings.ToLower(strings.TrimSpace(input.QueryType))

	legacyFields := 0
	if input.LicenseNo != "" {
		legacyFields++
	}
	if input.CardCode != "" {
		legacyFields++
	}
	if input.Query != "" {
		legacyFields++
	}
	if legacyFields != 1 {
		return PublicLicenseQueryResult{}, ErrInvalidRequest
	}
	if input.Query != "" {
		queryType := input.QueryType
		if queryType == "" || queryType == "auto" {
			queryType = detectPublicQueryType(input.Query)
		}
		switch queryType {
		case "license":
			input.LicenseNo = input.Query
		case "card":
			input.CardCode = input.Query
		case domain.BindDomain, domain.BindIP, domain.BindAccount:
			return s.queryPublicBindings(ctx, queryType, input.Query)
		default:
			return PublicLicenseQueryResult{}, ErrInvalidRequest
		}
	}
	if input.LicenseNo != "" {
		license, err := s.currentStore().GetLicenseByNo(ctx, input.LicenseNo)
		if err != nil {
			if store.IsNotFound(err) {
				return PublicLicenseQueryResult{}, ErrAuthorizationNotFound
			}
			return PublicLicenseQueryResult{}, err
		}
		return s.publicLicenseResult(ctx, "license", domain.Card{}, license)
	}

	card, err := s.currentStore().GetCardByHash(ctx, yncrypto.CardHash(s.cardPepper, input.CardCode))
	if err != nil {
		if store.IsNotFound(err) {
			return PublicLicenseQueryResult{}, ErrAuthorizationNotFound
		}
		return PublicLicenseQueryResult{}, err
	}
	if card.ActivatedLicenseID == "" {
		product, err := s.currentStore().GetProduct(ctx, card.ProductID)
		if err != nil {
			return PublicLicenseQueryResult{}, err
		}
		return PublicLicenseQueryResult{
			QueryType:    "card",
			ProductName:  product.Name,
			ProductCode:  product.Code,
			CardStatus:   card.Status,
			DurationDays: card.DurationDays,
			IsPermanent:  card.IsPermanent,
			ServerTime:   s.now(),
		}, nil
	}
	license, err := s.currentStore().GetLicenseByID(ctx, card.ActivatedLicenseID)
	if err != nil {
		if store.IsNotFound(err) {
			return PublicLicenseQueryResult{}, ErrAuthorizationNotFound
		}
		return PublicLicenseQueryResult{}, err
	}
	return s.publicLicenseResult(ctx, "card", card, license)
}

func (s *Service) queryPublicBindings(ctx context.Context, queryType, query string) (PublicLicenseQueryResult, error) {
	value, err := normalizeBindingValue(queryType, query)
	if err != nil {
		return PublicLicenseQueryResult{}, ErrInvalidRequest
	}
	bindings, err := s.currentStore().FindActiveBindingsByHash(ctx, queryType, yncrypto.BindHash(s.cardPepper, queryType, value), 10)
	if err != nil {
		return PublicLicenseQueryResult{}, err
	}
	if len(bindings) == 0 {
		return PublicLicenseQueryResult{}, ErrAuthorizationNotFound
	}
	matches := make([]PublicLicenseQueryMatch, 0, len(bindings))
	for _, binding := range bindings {
		license, err := s.currentStore().GetLicenseByID(ctx, binding.LicenseID)
		if err != nil {
			if store.IsNotFound(err) {
				continue
			}
			return PublicLicenseQueryResult{}, err
		}
		match, err := s.publicLicenseMatch(ctx, license)
		if err != nil {
			return PublicLicenseQueryResult{}, err
		}
		matches = append(matches, match)
	}
	if len(matches) == 0 {
		return PublicLicenseQueryResult{}, ErrAuthorizationNotFound
	}
	return PublicLicenseQueryResult{
		QueryType:   queryType,
		QueryMasked: maskPublicQuery(queryType, value),
		MatchCount:  len(matches),
		Results:     matches,
		ServerTime:  s.now(),
	}, nil
}

func (s *Service) publicLicenseMatch(ctx context.Context, license domain.License) (PublicLicenseQueryMatch, error) {
	product, err := s.currentStore().GetProduct(ctx, license.ProductID)
	if err != nil {
		return PublicLicenseQueryMatch{}, err
	}
	now := s.now()
	status := license.Status
	if status == domain.LicenseActive && license.ExpiredAt != nil && !now.Before(*license.ExpiredAt) {
		status = domain.LicenseExpired
	}
	activatedAt := license.ActivatedAt
	return PublicLicenseQueryMatch{
		ProductName: product.Name, ProductCode: product.Code, LicenseNo: license.LicenseNo,
		LicenseStatus: status, IsPermanent: license.ExpiredAt == nil, ActivatedAt: &activatedAt,
		ExpiredAt: license.ExpiredAt, LastVerifyAt: license.LastVerifyAt,
	}, nil
}

func (s *Service) publicLicenseResult(ctx context.Context, queryType string, card domain.Card, license domain.License) (PublicLicenseQueryResult, error) {
	product, err := s.currentStore().GetProduct(ctx, license.ProductID)
	if err != nil {
		return PublicLicenseQueryResult{}, err
	}
	now := s.now()
	status := license.Status
	if status == domain.LicenseActive && license.ExpiredAt != nil && !now.Before(*license.ExpiredAt) {
		status = domain.LicenseExpired
	}
	activatedAt := license.ActivatedAt
	result := PublicLicenseQueryResult{
		QueryType:     queryType,
		ProductName:   product.Name,
		ProductCode:   product.Code,
		LicenseNo:     license.LicenseNo,
		LicenseStatus: status,
		IsPermanent:   license.ExpiredAt == nil,
		ActivatedAt:   &activatedAt,
		ExpiredAt:     license.ExpiredAt,
		LastVerifyAt:  license.LastVerifyAt,
		ServerTime:    now,
	}
	if queryType == "card" {
		result.CardStatus = card.Status
		result.DurationDays = card.DurationDays
		result.IsPermanent = card.IsPermanent
	}
	return result, nil
}

type ActivateInput struct {
	AppKey        string `json:"app_key"`
	CardCode      string `json:"card_code"`
	BindMode      string `json:"bind_mode"`
	BindValue     string `json:"bind_value"`
	DeviceName    string `json:"device_name"`
	ClientVersion string `json:"client_version"`
	ClientIP      string `json:"-"`
	UserAgent     string `json:"-"`
}

type LicenseResponse struct {
	LicenseNo    string     `json:"license_no"`
	Status       string     `json:"status"`
	ExpiredAt    *time.Time `json:"expired_at,omitempty"`
	GraceUntil   *time.Time `json:"grace_until,omitempty"`
	LicenseToken string     `json:"license_token,omitempty"`
	OfflineToken string     `json:"offline_token,omitempty"`
	ServerTime   time.Time  `json:"server_time"`
}

func (s *Service) Activate(ctx context.Context, input ActivateInput) (LicenseResponse, error) {
	product, err := s.productByAppKey(ctx, input.AppKey)
	if err != nil {
		return LicenseResponse{}, err
	}
	if !yncrypto.ValidateChecksum(product.Code, input.CardCode) {
		return LicenseResponse{}, ErrCardInvalid
	}
	bindMode, bindValue, bindHash, err := s.normalizeBinding(product, input.BindMode, input.BindValue)
	if err != nil {
		return LicenseResponse{}, err
	}
	if err := s.checkRiskAccess(ctx, product.ID, input.ClientIP, bindMode, bindValue); err != nil {
		_ = s.auditClient(ctx, product.ID, "", "", "license.activate", "failed", errorCode(err), input.ClientIP, input.UserAgent)
		return LicenseResponse{}, err
	}
	now := s.now()
	codeHash := yncrypto.CardHash(s.cardPepper, input.CardCode)
	var lic domain.License
	var binding domain.Binding
	err = s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		card, err := tx.GetCardByHash(ctx, codeHash)
		if err != nil {
			if store.IsNotFound(err) {
				return ErrCardInvalid
			}
			return err
		}
		if card.ProductID != product.ID {
			return ErrCardInvalid
		}
		if card.Status == domain.CardVoided {
			return ErrCardVoided
		}
		if card.Status != domain.CardUnused && card.Status != domain.CardActivated {
			return ErrCardUsed
		}
		if card.ActivatedLicenseID == "" {
			lic = newLicense(product, card, now)
			if err := tx.CreateLicense(ctx, lic); err != nil {
				return err
			}
			if err := tx.UpdateCardActivated(ctx, card.ID, lic.ID, now); err != nil {
				return err
			}
		} else {
			lic, err = tx.GetLicenseByID(ctx, card.ActivatedLicenseID)
			if err != nil {
				return err
			}
		}
		binding, err = tx.ActiveBindingByHash(ctx, lic.ID, bindMode, bindHash)
		if err == nil {
			return nil
		}
		if !store.IsNotFound(err) {
			return err
		}
		count, err := tx.CountActiveBindings(ctx, lic.ID)
		if err != nil {
			return err
		}
		if product.MaxBindCount != -1 && count >= product.MaxBindCount {
			if product.BindConflictStrategy == domain.ConflictKickOldest {
				oldest, err := tx.OldestActiveBinding(ctx, lic.ID)
				if err != nil {
					return err
				}
				if err := tx.MarkBindingKicked(ctx, oldest.ID, now); err != nil {
					return err
				}
			} else {
				return ErrDeviceLimitExceeded
			}
		}
		encryptedBind, err := yncrypto.EncryptString(s.dataKey, bindValue)
		if err != nil {
			return err
		}
		binding = domain.Binding{
			ID:                 yncrypto.NewID("bind"),
			LicenseID:          lic.ID,
			ProductID:          product.ID,
			BindMode:           bindMode,
			BindValueHash:      bindHash,
			BindValueEncrypted: encryptedBind,
			DisplayName:        input.DeviceName,
			Status:             domain.BindingActive,
			FirstSeenIP:        input.ClientIP,
			LastSeenIP:         input.ClientIP,
			UserAgent:          input.UserAgent,
			ActivatedAt:        now,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		return tx.CreateBinding(ctx, binding)
	})
	if err != nil {
		_ = s.auditClient(ctx, product.ID, "", "", "license.activate", "failed", errorCode(err), input.ClientIP, input.UserAgent)
		s.evaluateFailedActivationRisk(ctx, product.ID, input.ClientIP)
		return LicenseResponse{}, err
	}
	_ = s.auditClient(ctx, product.ID, lic.ID, "", "license.activate", "success", "", input.ClientIP, input.UserAgent)
	s.evaluateActivationRisk(ctx, product.ID, lic, binding, input.ClientIP, bindValue)
	return s.licenseResponse(product, lic, binding, now)
}

type VerifyInput struct {
	AppKey       string `json:"app_key"`
	LicenseNo    string `json:"license_no"`
	BindMode     string `json:"bind_mode"`
	BindValue    string `json:"bind_value"`
	LicenseToken string `json:"license_token"`
	ClientIP     string `json:"-"`
	UserAgent    string `json:"-"`
}

func (s *Service) Verify(ctx context.Context, input VerifyInput) (LicenseResponse, error) {
	product, err := s.productByAppKey(ctx, input.AppKey)
	if err != nil {
		return LicenseResponse{}, err
	}
	bindMode, bindValue, bindHash, err := s.normalizeBinding(product, input.BindMode, input.BindValue)
	if err != nil {
		return LicenseResponse{}, err
	}
	if err := s.checkRiskAccess(ctx, product.ID, input.ClientIP, bindMode, bindValue); err != nil {
		_ = s.auditClient(ctx, product.ID, "", "", "license.verify", "failed", errorCode(err), input.ClientIP, input.UserAgent)
		return LicenseResponse{}, err
	}
	now := s.now()
	var lic domain.License
	var binding domain.Binding
	err = s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		var err error
		lic, err = tx.GetLicenseByNo(ctx, input.LicenseNo)
		if err != nil {
			if store.IsNotFound(err) {
				return ErrLicenseNotFound
			}
			return err
		}
		if lic.ProductID != product.ID {
			return ErrLicenseNotFound
		}
		if lic.Status == domain.LicenseRevoked {
			return ErrLicenseRevoked
		}
		binding, err = tx.ActiveBindingByHash(ctx, lic.ID, bindMode, bindHash)
		if err != nil {
			if store.IsNotFound(err) {
				return ErrBindingMismatch
			}
			return err
		}
		if err := s.ensureUsable(product, lic, now); err != nil {
			return err
		}
		return tx.UpdateLicenseVerify(ctx, lic.ID, now)
	})
	if err != nil {
		_ = s.auditClient(ctx, product.ID, "", "", "license.verify", "failed", errorCode(err), input.ClientIP, input.UserAgent)
		return LicenseResponse{}, err
	}
	_ = s.auditClient(ctx, product.ID, lic.ID, "", "license.verify", "success", "", input.ClientIP, input.UserAgent)
	return s.licenseResponse(product, lic, binding, now)
}

func (s *Service) Heartbeat(ctx context.Context, input VerifyInput) (map[string]any, error) {
	product, err := s.productByAppKey(ctx, input.AppKey)
	if err != nil {
		return nil, err
	}
	bindMode, bindValue, bindHash, err := s.normalizeBinding(product, input.BindMode, input.BindValue)
	if err != nil {
		return nil, err
	}
	if err := s.checkRiskAccess(ctx, product.ID, input.ClientIP, bindMode, bindValue); err != nil {
		_ = s.auditClient(ctx, product.ID, "", "", "license.heartbeat", "failed", errorCode(err), input.ClientIP, input.UserAgent)
		return nil, err
	}
	now := s.now()
	err = s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		lic, err := tx.GetLicenseByNo(ctx, input.LicenseNo)
		if err != nil {
			if store.IsNotFound(err) {
				return ErrLicenseNotFound
			}
			return err
		}
		binding, err := tx.ActiveBindingByHash(ctx, lic.ID, bindMode, bindHash)
		if err != nil {
			if store.IsNotFound(err) {
				return ErrBindingMismatch
			}
			return err
		}
		if err := s.ensureUsable(product, lic, now); err != nil {
			return err
		}
		return tx.UpdateHeartbeat(ctx, lic.ID, binding.ID, now, input.ClientIP)
	})
	if err != nil {
		_ = s.auditClient(ctx, product.ID, "", "", "license.heartbeat", "failed", errorCode(err), input.ClientIP, input.UserAgent)
		return nil, err
	}
	_ = s.auditClient(ctx, product.ID, "", "", "license.heartbeat", "success", "", input.ClientIP, input.UserAgent)
	return map[string]any{"accepted": true, "server_time": now}, nil
}

type RenewInput struct {
	AppKey        string `json:"app_key"`
	LicenseNo     string `json:"license_no"`
	RenewCardCode string `json:"renew_card_code"`
	BindMode      string `json:"bind_mode"`
	BindValue     string `json:"bind_value"`
	ClientIP      string `json:"-"`
	UserAgent     string `json:"-"`
}

func (s *Service) Renew(ctx context.Context, input RenewInput) (LicenseResponse, error) {
	product, err := s.productByAppKey(ctx, input.AppKey)
	if err != nil {
		return LicenseResponse{}, err
	}
	bindMode, bindValue, bindHash, err := s.normalizeBinding(product, input.BindMode, input.BindValue)
	if err != nil {
		return LicenseResponse{}, err
	}
	if err := s.checkRiskAccess(ctx, product.ID, input.ClientIP, bindMode, bindValue); err != nil {
		_ = s.auditClient(ctx, product.ID, "", "", "license.renew", "failed", errorCode(err), input.ClientIP, input.UserAgent)
		return LicenseResponse{}, err
	}
	if !yncrypto.ValidateChecksum(product.Code, input.RenewCardCode) {
		return LicenseResponse{}, ErrCardInvalid
	}
	now := s.now()
	codeHash := yncrypto.CardHash(s.cardPepper, input.RenewCardCode)
	var lic domain.License
	var binding domain.Binding
	err = s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		var err error
		lic, err = tx.GetLicenseByNo(ctx, input.LicenseNo)
		if err != nil {
			if store.IsNotFound(err) {
				return ErrLicenseNotFound
			}
			return err
		}
		if lic.ExpiredAt == nil {
			return ErrLicensePermanent
		}
		binding, err = tx.ActiveBindingByHash(ctx, lic.ID, bindMode, bindHash)
		if err != nil {
			if store.IsNotFound(err) {
				return ErrBindingMismatch
			}
			return err
		}
		card, err := tx.GetCardByHash(ctx, codeHash)
		if err != nil {
			if store.IsNotFound(err) {
				return ErrCardInvalid
			}
			return err
		}
		if card.ProductID != product.ID || card.Status != domain.CardUnused {
			return ErrCardInvalid
		}
		var newExpiry *time.Time
		if !card.IsPermanent {
			base := now
			if lic.ExpiredAt != nil && lic.ExpiredAt.After(now) {
				base = *lic.ExpiredAt
			}
			next := base.AddDate(0, 0, card.DurationDays)
			newExpiry = &next
		}
		if err := tx.UpdateLicenseExpiry(ctx, lic.ID, newExpiry, now); err != nil {
			return err
		}
		if err := tx.UpdateCardConsumed(ctx, card.ID, now); err != nil {
			return err
		}
		lic.ExpiredAt = newExpiry
		lic.Status = domain.LicenseActive
		return nil
	})
	if err != nil {
		_ = s.auditClient(ctx, product.ID, "", "", "license.renew", "failed", errorCode(err), input.ClientIP, input.UserAgent)
		return LicenseResponse{}, err
	}
	_ = s.auditClient(ctx, product.ID, lic.ID, "", "license.renew", "success", "", input.ClientIP, input.UserAgent)
	return s.licenseResponse(product, lic, binding, now)
}

type UnbindInput struct {
	AppKey    string `json:"app_key"`
	LicenseNo string `json:"license_no"`
	BindMode  string `json:"bind_mode"`
	BindValue string `json:"bind_value"`
	Reason    string `json:"reason"`
	ClientIP  string `json:"-"`
	UserAgent string `json:"-"`
}

func (s *Service) Unbind(ctx context.Context, input UnbindInput) (map[string]any, error) {
	product, err := s.productByAppKey(ctx, input.AppKey)
	if err != nil {
		return nil, err
	}
	bindMode, bindValue, bindHash, err := s.normalizeBinding(product, input.BindMode, input.BindValue)
	if err != nil {
		return nil, err
	}
	if err := s.checkRiskAccess(ctx, product.ID, input.ClientIP, bindMode, bindValue); err != nil {
		_ = s.auditClient(ctx, product.ID, "", "", "license.unbind", "failed", errorCode(err), input.ClientIP, input.UserAgent)
		return nil, err
	}
	now := s.now()
	var lic domain.License
	var binding domain.Binding
	err = s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		var err error
		lic, err = tx.GetLicenseByNo(ctx, input.LicenseNo)
		if err != nil {
			if store.IsNotFound(err) {
				return ErrLicenseNotFound
			}
			return err
		}
		if lic.ProductID != product.ID {
			return ErrLicenseNotFound
		}
		if lic.Status == domain.LicenseRevoked {
			return ErrLicenseRevoked
		}
		binding, err = tx.ActiveBindingByHash(ctx, lic.ID, bindMode, bindHash)
		if err != nil {
			if store.IsNotFound(err) {
				return ErrBindingMismatch
			}
			return err
		}
		return tx.MarkBindingUnbound(ctx, binding.ID, now)
	})
	if err != nil {
		_ = s.auditClient(ctx, product.ID, "", "", "license.unbind", "failed", errorCode(err), input.ClientIP, input.UserAgent)
		return nil, err
	}
	_ = s.auditClient(ctx, product.ID, lic.ID, "", "license.unbind", "success", "", input.ClientIP, input.UserAgent)
	return map[string]any{"unbound": true, "license_no": lic.LicenseNo, "server_time": now}, nil
}

type RevokeLicenseInput struct {
	LicenseNo string `json:"license_no"`
	Reason    string `json:"reason"`
	ActorID   string `json:"-"`
}

type AdminUnbindInput struct {
	LicenseNo string `json:"license_no"`
	BindingID string `json:"binding_id"`
	Reason    string `json:"reason"`
	ActorID   string `json:"-"`
}

func (s *Service) AdminUnbind(ctx context.Context, input AdminUnbindInput) (map[string]any, error) {
	if strings.TrimSpace(input.LicenseNo) == "" || strings.TrimSpace(input.BindingID) == "" {
		return nil, ErrInvalidRequest
	}
	now := s.now()
	var lic domain.License
	err := s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		var err error
		lic, err = tx.GetLicenseByNo(ctx, input.LicenseNo)
		if err != nil {
			if store.IsNotFound(err) {
				return ErrLicenseNotFound
			}
			return err
		}
		binding, err := tx.GetBindingByID(ctx, input.BindingID)
		if err != nil {
			if store.IsNotFound(err) {
				return ErrBindingMismatch
			}
			return err
		}
		if binding.LicenseID != lic.ID {
			return ErrBindingMismatch
		}
		if binding.Status != domain.BindingActive {
			return nil
		}
		return tx.MarkBindingUnbound(ctx, binding.ID, now)
	})
	if err != nil {
		return nil, err
	}
	_ = s.audit(ctx, "admin", input.ActorID, lic.ProductID, lic.ID, "", "license.admin_unbind", "success", "")
	return map[string]any{"unbound": true, "license_no": lic.LicenseNo, "binding_id": input.BindingID, "server_time": now}, nil
}

func (s *Service) RevokeLicense(ctx context.Context, input RevokeLicenseInput) (domain.License, error) {
	if strings.TrimSpace(input.LicenseNo) == "" {
		return domain.License{}, ErrInvalidRequest
	}
	now := s.now()
	var lic domain.License
	err := s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		var err error
		lic, err = tx.GetLicenseByNo(ctx, input.LicenseNo)
		if err != nil {
			if store.IsNotFound(err) {
				return ErrLicenseNotFound
			}
			return err
		}
		if lic.Status == domain.LicenseRevoked {
			return nil
		}
		if err := tx.RevokeLicense(ctx, lic.ID, input.Reason, now); err != nil {
			return err
		}
		lic.Status = domain.LicenseRevoked
		lic.UpdatedAt = now
		return nil
	})
	if err != nil {
		return domain.License{}, err
	}
	_ = s.audit(ctx, "admin", input.ActorID, lic.ProductID, lic.ID, "", "license.revoke", "success", "")
	return lic, nil
}

func (s *Service) productByAppKey(ctx context.Context, appKey string) (domain.Product, error) {
	product, err := s.currentStore().GetProductByAppKey(ctx, appKey)
	if err != nil {
		if store.IsNotFound(err) {
			return domain.Product{}, ErrInvalidAppKey
		}
		return domain.Product{}, err
	}
	if product.Status != domain.ProductActive {
		return domain.Product{}, ErrProductDisabled
	}
	return product, nil
}

func (s *Service) normalizeBinding(product domain.Product, inputMode, inputValue string) (string, string, string, error) {
	mode := product.BindMode
	if inputMode != "" {
		mode = inputMode
	}
	if mode == "" {
		mode = domain.BindNone
	}
	value := strings.TrimSpace(inputValue)
	if mode == domain.BindNone {
		value = "none"
	}
	if value == "" {
		return "", "", "", ErrBindingRequired
	}
	if mode != product.BindMode {
		return "", "", "", ErrBindingMismatch
	}
	value, err := normalizeBindingValue(mode, value)
	if err != nil {
		return "", "", "", ErrBindingRequired
	}
	return mode, value, yncrypto.BindHash(s.cardPepper, mode, value), nil
}

func normalizeBindingValue(mode, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ErrBindingRequired
	}
	switch mode {
	case domain.BindIP:
		ip := net.ParseIP(value)
		if ip == nil {
			return "", ErrBindingRequired
		}
		return ip.String(), nil
	case domain.BindDomain:
		host := strings.ToLower(value)
		if strings.Contains(host, "://") {
			parsed, err := url.Parse(host)
			if err != nil {
				return "", ErrBindingRequired
			}
			host = parsed.Hostname()
		} else {
			host = strings.Split(host, "/")[0]
			host = strings.Split(host, ":")[0]
		}
		host = strings.TrimSuffix(host, ".")
		if !isValidDomain(host) {
			return "", ErrBindingRequired
		}
		return host, nil
	case domain.BindAccount:
		return strings.ToLower(value), nil
	default:
		return value, nil
	}
}

func detectPublicQueryType(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(trimmed), "lic_") {
		return "license"
	}
	if net.ParseIP(trimmed) != nil {
		return domain.BindIP
	}
	if strings.Contains(trimmed, ".") && !strings.ContainsAny(trimmed, " @") {
		return domain.BindDomain
	}
	if strings.Count(trimmed, "-") >= 2 {
		return "card"
	}
	return domain.BindAccount
}

func maskPublicQuery(queryType, value string) string {
	switch queryType {
	case domain.BindIP:
		if ip := net.ParseIP(value); ip != nil {
			if v4 := ip.To4(); v4 != nil {
				return v4.String()[:strings.LastIndex(v4.String(), ".")+1] + "*"
			}
			parts := strings.Split(ip.String(), ":")
			if len(parts) > 2 {
				return strings.Join(parts[:2], ":") + "::*"
			}
		}
	case domain.BindDomain:
		parts := strings.Split(value, ".")
		return maskText(parts[0]) + "." + strings.Join(parts[1:], ".")
	case domain.BindAccount:
		if at := strings.IndexByte(value, '@'); at > 0 {
			return maskText(value[:at]) + value[at:]
		}
		if len(value) == 11 && isASCIIDigits(value) {
			return value[:3] + "****" + value[7:]
		}
	}
	return maskText(value)
}

func maskText(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return "*"
	}
	if len(runes) <= 2 {
		return string(runes[:1]) + "*"
	}
	if len(runes) <= 6 {
		return string(runes[:1]) + strings.Repeat("*", len(runes)-2) + string(runes[len(runes)-1:])
	}
	return string(runes[:3]) + strings.Repeat("*", min(4, len(runes)-5)) + string(runes[len(runes)-2:])
}

func isValidDomain(value string) bool {
	if len(value) > 253 || net.ParseIP(value) != nil || !strings.Contains(value, ".") {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return false
			}
		}
	}
	return true
}

func isASCIIDigits(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return value != ""
}

func (s *Service) ensureUsable(product domain.Product, lic domain.License, now time.Time) error {
	if lic.Status == domain.LicenseRevoked {
		return ErrLicenseRevoked
	}
	if lic.ExpiredAt == nil {
		return nil
	}
	graceUntil := lic.ExpiredAt.AddDate(0, 0, product.ExpireGraceDays)
	if now.After(graceUntil) {
		return ErrLicenseExpired
	}
	return nil
}

func (s *Service) licenseResponse(product domain.Product, lic domain.License, binding domain.Binding, now time.Time) (LicenseResponse, error) {
	privatePEM, err := yncrypto.DecryptString(s.dataKey, product.PrivateKeyEncrypted)
	if err != nil {
		return LicenseResponse{}, err
	}
	bindValue, err := yncrypto.DecryptString(s.dataKey, binding.BindValueEncrypted)
	if err != nil {
		return LicenseResponse{}, err
	}
	bindDigest := yncrypto.BindDigest(binding.BindMode, bindValue)
	graceUntil := (*time.Time)(nil)
	if lic.ExpiredAt != nil {
		t := lic.ExpiredAt.AddDate(0, 0, product.ExpireGraceDays)
		graceUntil = &t
	}
	payload := map[string]any{
		"type":         "license",
		"license_no":   lic.LicenseNo,
		"product_code": product.Code,
		"bind_mode":    binding.BindMode,
		"bind_hash":    binding.BindValueHash,
		"bind_digest":  bindDigest,
		"status":       lic.Status,
		"issued_at":    now,
		"expired_at":   lic.ExpiredAt,
		"key_version":  product.KeyVersion,
	}
	licenseToken, err := yncrypto.SignJSON(privatePEM, payload)
	if err != nil {
		return LicenseResponse{}, err
	}
	offlineUntil := now.AddDate(0, 0, product.OfflineGraceDays)
	if lic.ExpiredAt != nil && lic.ExpiredAt.Before(offlineUntil) {
		offlineUntil = *lic.ExpiredAt
	}
	offlinePayload := map[string]any{
		"type":               "offline_cache",
		"license_no":         lic.LicenseNo,
		"product_code":       product.Code,
		"bind_mode":          binding.BindMode,
		"bind_hash":          binding.BindValueHash,
		"bind_digest":        bindDigest,
		"license_expired_at": lic.ExpiredAt,
		"offline_until":      offlineUntil,
		"issued_at":          now,
		"key_version":        product.KeyVersion,
	}
	offlineToken, err := yncrypto.SignJSON(privatePEM, offlinePayload)
	if err != nil {
		return LicenseResponse{}, err
	}
	return LicenseResponse{
		LicenseNo:    lic.LicenseNo,
		Status:       lic.Status,
		ExpiredAt:    lic.ExpiredAt,
		GraceUntil:   graceUntil,
		LicenseToken: licenseToken,
		OfflineToken: offlineToken,
		ServerTime:   now,
	}, nil
}

func newLicense(product domain.Product, card domain.Card, now time.Time) domain.License {
	var expiredAt *time.Time
	if !card.IsPermanent {
		t := now.AddDate(0, 0, card.DurationDays)
		expiredAt = &t
	}
	return domain.License{
		ID:                  yncrypto.NewID("licid"),
		LicenseNo:           yncrypto.NewID("lic"),
		ProductID:           product.ID,
		CardID:              card.ID,
		AgentID:             card.AgentID,
		Status:              domain.LicenseActive,
		IssuedAt:            now,
		ActivatedAt:         now,
		ExpiredAt:           expiredAt,
		OfflineTokenVersion: 1,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
}

func (s *Service) buildCardBatch(product domain.Product, agentID, name string, quantity, durationDays int, isPermanent bool, source string) (domain.CardBatch, []domain.Card, []string, error) {
	now := s.now()
	batch := domain.CardBatch{
		ID:           yncrypto.NewID("batch"),
		ProductID:    product.ID,
		AgentID:      agentID,
		Name:         name,
		Quantity:     quantity,
		DurationDays: durationDays,
		IsPermanent:  isPermanent,
		Source:       source,
		Status:       "active",
		CreatedBy:    agentID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	codes := make([]string, 0, quantity)
	cards := make([]domain.Card, 0, quantity)
	seen := map[string]struct{}{}
	for len(cards) < quantity {
		code, err := yncrypto.GenerateCardCode(product.Code)
		if err != nil {
			return domain.CardBatch{}, nil, nil, err
		}
		hash := yncrypto.CardHash(s.cardPepper, code)
		if _, ok := seen[hash]; ok {
			continue
		}
		seen[hash] = struct{}{}
		encrypted, err := yncrypto.EncryptString(s.dataKey, yncrypto.NormalizeCardCode(code))
		if err != nil {
			return domain.CardBatch{}, nil, nil, err
		}
		codes = append(codes, code)
		cards = append(cards, domain.Card{
			ID:            yncrypto.NewID("card"),
			ProductID:     product.ID,
			BatchID:       batch.ID,
			AgentID:       agentID,
			CodeHash:      hash,
			CodeEncrypted: encrypted,
			CodePrefix:    product.Code,
			DurationDays:  durationDays,
			IsPermanent:   isPermanent,
			Status:        domain.CardUnused,
			CreatedBy:     agentID,
			CreatedAt:     now,
			UpdatedAt:     now,
		})
	}
	return batch, cards, codes, nil
}

func ensureAgentActive(agent domain.Agent) error {
	switch agent.Status {
	case domain.AgentActive:
		return nil
	case domain.AgentSuspended:
		return ErrAgentSuspended
	default:
		return ErrAgentDisabled
	}
}

func validateAgentPolicy(policy domain.AgentProductPolicy, quantity, durationDays int, isPermanent bool) error {
	if policy.Status != domain.AgentPolicyActive || !policy.CanGenerate {
		return ErrAgentProductDenied
	}
	if policy.MaxBatchQuantity > 0 && quantity > policy.MaxBatchQuantity {
		return ErrAgentBatchExceeded
	}
	if isPermanent {
		if !policy.AllowPermanent {
			return ErrAgentPermanentDenied
		}
		return nil
	}
	if len(policy.AllowedDurationDays) == 0 {
		return nil
	}
	for _, allowed := range policy.AllowedDurationDays {
		if allowed == durationDays {
			return nil
		}
	}
	return ErrAgentDurationDenied
}

func (s *Service) audit(ctx context.Context, actorType, actorID, productID, licenseID, cardID, action, result, code string) error {
	return s.currentStore().InsertAudit(ctx, domain.AuditLog{
		ID:        yncrypto.NewID("audit"),
		ActorType: actorType,
		ActorID:   actorID,
		ProductID: productID,
		LicenseID: licenseID,
		CardID:    cardID,
		Action:    action,
		Result:    result,
		ErrorCode: code,
		CreatedAt: s.now(),
	})
}

func (s *Service) auditAgent(ctx context.Context, actorID, agentID, productID, action, result, code string) error {
	return s.currentStore().InsertAudit(ctx, domain.AuditLog{
		ID:        yncrypto.NewID("audit"),
		ActorType: "agent",
		ActorID:   actorID,
		AgentID:   agentID,
		ProductID: productID,
		Action:    action,
		Result:    result,
		ErrorCode: code,
		CreatedAt: s.now(),
	})
}

func (s *Service) auditAdminAgent(ctx context.Context, actorID, agentID, action, result, code string) error {
	return s.currentStore().InsertAudit(ctx, domain.AuditLog{
		ID:        yncrypto.NewID("audit"),
		ActorType: "admin",
		ActorID:   actorID,
		AgentID:   agentID,
		Action:    action,
		Result:    result,
		ErrorCode: code,
		CreatedAt: s.now(),
	})
}

func errorCode(err error) string {
	var appErr AppError
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return "INTERNAL_ERROR"
}

func nonEmpty(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func IsSQLConflict(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
