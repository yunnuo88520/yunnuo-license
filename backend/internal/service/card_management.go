package service

import (
	"context"
	"strings"

	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

func (s *Service) ChangeProductStatus(ctx context.Context, productID, status, actorID string) (domain.Product, error) {
	productID = strings.TrimSpace(productID)
	if productID == "" || (status != domain.ProductActive && status != domain.ProductDisabled) {
		return domain.Product{}, ErrInvalidRequest
	}
	product, err := s.currentStore().GetProduct(ctx, productID)
	if err != nil {
		if store.IsNotFound(err) {
			return domain.Product{}, ErrProductNotFound
		}
		return domain.Product{}, err
	}
	now := s.now()
	if err := s.currentStore().UpdateProductStatus(ctx, productID, status, now); err != nil {
		return domain.Product{}, err
	}
	product.Status = status
	product.UpdatedAt = now
	_ = s.audit(ctx, "admin", actorID, product.ID, "", "", "product.status."+status, "success", "")
	return product, nil
}

type UpdateProductInput struct {
	Name                 string `json:"name"`
	BindMode             string `json:"bind_mode"`
	MaxBindCount         int    `json:"max_bind_count"`
	BindConflictStrategy string `json:"bind_conflict_strategy"`
	OfflineGraceDays     int    `json:"offline_grace_days"`
}

// UpdateProductProfile edits presentation and binding settings. License mode,
// update window and expire grace stay under the locked license-policy endpoint.
func (s *Service) UpdateProductProfile(ctx context.Context, productID string, input UpdateProductInput, actorID string) (domain.Product, error) {
	productID = strings.TrimSpace(productID)
	input.Name = strings.TrimSpace(input.Name)
	if productID == "" || input.Name == "" {
		return domain.Product{}, ErrInvalidRequest
	}
	switch input.BindMode {
	case domain.BindNone, domain.BindDevice, domain.BindDomain, domain.BindIP, domain.BindAccount:
	default:
		return domain.Product{}, ErrInvalidRequest
	}
	if input.BindConflictStrategy == "" {
		input.BindConflictStrategy = domain.ConflictReject
	}
	if input.BindConflictStrategy != domain.ConflictReject && input.BindConflictStrategy != domain.ConflictKickOldest {
		return domain.Product{}, ErrInvalidRequest
	}
	if input.MaxBindCount <= 0 {
		input.MaxBindCount = 1
	}
	if input.OfflineGraceDays < 0 || input.OfflineGraceDays > maxOfflineGraceDays {
		return domain.Product{}, badRequest("OFFLINE_GRACE_TOO_LONG", "离线缓存宽限最长 365 天")
	}
	product, err := s.currentStore().GetProduct(ctx, productID)
	if err != nil {
		if store.IsNotFound(err) {
			return domain.Product{}, ErrProductNotFound
		}
		return domain.Product{}, err
	}
	now := s.now()
	if err := s.currentStore().UpdateProductProfile(ctx, productID, input.Name,
		input.BindMode, input.BindConflictStrategy, input.MaxBindCount, input.OfflineGraceDays, now); err != nil {
		return domain.Product{}, err
	}
	product.Name = input.Name
	product.BindMode = input.BindMode
	product.BindConflictStrategy = input.BindConflictStrategy
	product.MaxBindCount = input.MaxBindCount
	product.OfflineGraceDays = input.OfflineGraceDays
	product.UpdatedAt = now
	_ = s.audit(ctx, "admin", actorID, product.ID, "", "", "product.update", "success", "")
	return product, nil
}

type CardBatchExportResult struct {
	Batch domain.CardBatch `json:"batch"`
	Codes []string         `json:"codes"`
}

func (s *Service) ExportCardBatch(ctx context.Context, batchID, actorID string) (CardBatchExportResult, error) {
	batchID = strings.TrimSpace(batchID)
	if batchID == "" {
		return CardBatchExportResult{}, ErrInvalidRequest
	}
	batch, err := s.currentStore().GetCardBatch(ctx, batchID)
	if err != nil {
		if store.IsNotFound(err) {
			return CardBatchExportResult{}, ErrCardBatchNotFound
		}
		return CardBatchExportResult{}, err
	}
	cards, err := s.currentStore().ListCardsByBatch(ctx, batchID)
	if err != nil {
		return CardBatchExportResult{}, err
	}
	codes := make([]string, 0, len(cards))
	for _, card := range cards {
		code, err := yncrypto.DecryptString(s.dataKey, card.CodeEncrypted)
		if err != nil {
			return CardBatchExportResult{}, err
		}
		codes = append(codes, code)
	}
	now := s.now()
	if err := s.currentStore().IncrementCardBatchExportCount(ctx, batchID, now); err != nil {
		return CardBatchExportResult{}, err
	}
	batch.ExportCount++
	batch.UpdatedAt = now
	_ = s.audit(ctx, "admin", actorID, batch.ProductID, "", "", "card_batch.export", "success", "")
	return CardBatchExportResult{Batch: batch, Codes: codes}, nil
}

type VoidCardBatchInput struct {
	Reason  string `json:"reason"`
	ActorID string `json:"-"`
}

// VoidCardBatch 整批作废未使用卡密（已激活卡不受影响），用于批次泄漏止损。
func (s *Service) VoidCardBatch(ctx context.Context, batchID string, input VoidCardBatchInput) (map[string]any, error) {
	batchID = strings.TrimSpace(batchID)
	reason := strings.TrimSpace(input.Reason)
	if batchID == "" || reason == "" || len(reason) > 200 {
		return nil, ErrInvalidRequest
	}
	batch, err := s.currentStore().GetCardBatch(ctx, batchID)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, ErrCardBatchNotFound
		}
		return nil, err
	}
	// 批次状态与卡密状态独立维护：作废只改卡密，不改批次元数据。
	voided, err := s.currentStore().VoidUnusedCardsInBatch(ctx, batchID, reason, s.now())
	if err != nil {
		return nil, err
	}
	_ = s.audit(ctx, "admin", input.ActorID, batch.ProductID, "", batchID, "card_batch.void_unused", "success", reason)
	return map[string]any{"voided": voided, "batch_id": batchID}, nil
}

func (s *Service) VoidCard(ctx context.Context, cardID, reason, actorID string) (domain.Card, error) {
	cardID = strings.TrimSpace(cardID)
	reason = strings.TrimSpace(reason)
	if cardID == "" || reason == "" || len(reason) > 200 {
		return domain.Card{}, ErrInvalidRequest
	}
	card, err := s.currentStore().GetCard(ctx, cardID)
	if err != nil {
		if store.IsNotFound(err) {
			return domain.Card{}, ErrCardNotFound
		}
		return domain.Card{}, err
	}
	switch card.Status {
	case domain.CardVoided:
		return card, nil
	case domain.CardUnused:
	default:
		return domain.Card{}, ErrCardCannotVoid
	}
	now := s.now()
	if err := s.currentStore().UpdateCardVoided(ctx, cardID, reason, now); err != nil {
		if store.IsNotFound(err) {
			return domain.Card{}, ErrCardCannotVoid
		}
		return domain.Card{}, err
	}
	card.Status = domain.CardVoided
	card.VoidReason = reason
	card.VoidedAt = &now
	card.UpdatedAt = now
	_ = s.audit(ctx, "admin", actorID, card.ProductID, "", card.ID, "card.void", "success", "")
	return card, nil
}
