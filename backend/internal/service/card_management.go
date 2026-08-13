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
	product, err := s.store.GetProduct(ctx, productID)
	if err != nil {
		if store.IsNotFound(err) {
			return domain.Product{}, ErrProductNotFound
		}
		return domain.Product{}, err
	}
	now := s.now()
	if err := s.store.UpdateProductStatus(ctx, productID, status, now); err != nil {
		return domain.Product{}, err
	}
	product.Status = status
	product.UpdatedAt = now
	_ = s.audit(ctx, "admin", actorID, product.ID, "", "", "product.status."+status, "success", "")
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
	batch, err := s.store.GetCardBatch(ctx, batchID)
	if err != nil {
		if store.IsNotFound(err) {
			return CardBatchExportResult{}, ErrCardBatchNotFound
		}
		return CardBatchExportResult{}, err
	}
	cards, err := s.store.ListCardsByBatch(ctx, batchID)
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
	if err := s.store.IncrementCardBatchExportCount(ctx, batchID, now); err != nil {
		return CardBatchExportResult{}, err
	}
	batch.ExportCount++
	batch.UpdatedAt = now
	_ = s.audit(ctx, "admin", actorID, batch.ProductID, "", "", "card_batch.export", "success", "")
	return CardBatchExportResult{Batch: batch, Codes: codes}, nil
}

func (s *Service) VoidCard(ctx context.Context, cardID, reason, actorID string) (domain.Card, error) {
	cardID = strings.TrimSpace(cardID)
	reason = strings.TrimSpace(reason)
	if cardID == "" || len(reason) > 200 {
		return domain.Card{}, ErrInvalidRequest
	}
	if reason == "" {
		reason = "admin_void"
	}
	card, err := s.store.GetCard(ctx, cardID)
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
	if err := s.store.UpdateCardVoided(ctx, cardID, reason, now); err != nil {
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
