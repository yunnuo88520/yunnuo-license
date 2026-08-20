package service

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

type CreateRiskBlockInput struct {
	ProductID string `json:"product_id"`
	Kind      string `json:"kind"`
	Value     string `json:"value"`
	Reason    string `json:"reason"`
	ActorID   string `json:"-"`
}

type RiskBlockFilter struct {
	Kind      string
	Status    string
	ProductID string
}

type RiskAlertFilter struct {
	Status    string
	Severity  string
	AlertType string
	ProductID string
	Limit     int
}

func (s *Service) CreateRiskBlock(ctx context.Context, input CreateRiskBlockInput) (domain.RiskBlock, error) {
	input.ProductID = strings.TrimSpace(input.ProductID)
	kind := strings.ToLower(strings.TrimSpace(input.Kind))
	value, err := normalizeRiskValue(kind, input.Value)
	reason := strings.TrimSpace(input.Reason)
	if err != nil || reason == "" || len(reason) > 500 {
		return domain.RiskBlock{}, ErrInvalidRequest
	}
	if input.ProductID != "" {
		if _, err := s.currentStore().GetProduct(ctx, input.ProductID); err != nil {
			if store.IsNotFound(err) {
				return domain.RiskBlock{}, ErrProductNotFound
			}
			return domain.RiskBlock{}, err
		}
	}
	now := s.now()
	block := domain.RiskBlock{
		ID:          yncrypto.NewID("rblock"),
		ProductID:   input.ProductID,
		Kind:        kind,
		ValueHash:   s.riskHash(kind, value),
		ValueMasked: maskRiskValue(kind, value),
		Reason:      reason,
		Status:      domain.RiskStatusActive,
		CreatedBy:   input.ActorID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.currentStore().CreateRiskBlock(ctx, block); err != nil {
		return domain.RiskBlock{}, err
	}
	_ = s.audit(ctx, "admin", input.ActorID, input.ProductID, "", "", "risk_block.create", "success", "")
	persisted, err := s.currentStore().FindActiveRiskBlock(ctx, input.ProductID, kind, block.ValueHash)
	if err != nil {
		return domain.RiskBlock{}, err
	}
	return persisted, nil
}

func (s *Service) ListRiskBlocks(ctx context.Context, filter RiskBlockFilter) ([]domain.RiskBlock, error) {
	if filter.Kind != "" && filter.Kind != domain.RiskBlockIP && filter.Kind != domain.RiskBlockDevice {
		return nil, ErrInvalidRequest
	}
	if filter.Status != "" && filter.Status != domain.RiskStatusActive && filter.Status != domain.RiskStatusDisabled {
		return nil, ErrInvalidRequest
	}
	return s.currentStore().ListRiskBlocks(ctx, filter.Kind, filter.Status, filter.ProductID)
}

func (s *Service) DisableRiskBlock(ctx context.Context, id, actorID string) error {
	if strings.TrimSpace(id) == "" {
		return ErrInvalidRequest
	}
	if err := s.currentStore().DisableRiskBlock(ctx, id, s.now()); err != nil {
		if store.IsNotFound(err) {
			return ErrRiskBlockNotFound
		}
		return err
	}
	_ = s.audit(ctx, "admin", actorID, "", "", "", "risk_block.disable", "success", "")
	return nil
}

func (s *Service) ListRiskAlerts(ctx context.Context, filter RiskAlertFilter) ([]domain.RiskAlert, error) {
	if filter.Limit <= 0 {
		filter.Limit = 100
	}
	if filter.Limit > 500 {
		filter.Limit = 500
	}
	return s.currentStore().ListRiskAlerts(ctx, filter.Status, filter.Severity, filter.AlertType, filter.ProductID, filter.Limit)
}

func (s *Service) ResolveRiskAlert(ctx context.Context, id, actorID string) error {
	if strings.TrimSpace(id) == "" {
		return ErrInvalidRequest
	}
	if err := s.currentStore().ResolveRiskAlert(ctx, id, actorID, s.now()); err != nil {
		if store.IsNotFound(err) {
			return ErrRiskAlertNotFound
		}
		return err
	}
	_ = s.audit(ctx, "admin", actorID, "", "", "", "risk_alert.resolve", "success", "")
	return nil
}

func (s *Service) RiskSummary(ctx context.Context) (domain.RiskSummary, error) {
	return s.currentStore().RiskSummary(ctx, s.now().Add(-24*time.Hour))
}

func (s *Service) checkRiskAccess(ctx context.Context, productID, clientIP, bindMode, bindValue string) error {
	if ip, err := normalizeRiskValue(domain.RiskBlockIP, clientIP); err == nil {
		if _, err := s.currentStore().FindActiveRiskBlock(ctx, productID, domain.RiskBlockIP, s.riskHash(domain.RiskBlockIP, ip)); err == nil {
			s.recordRiskAlert(ctx, productID, "", "", "blocked_ip", "critical", domain.RiskBlockIP, ip, "黑名单 IP 尝试访问授权接口")
			return ErrRiskBlockedIP
		} else if !store.IsNotFound(err) {
			return err
		}
	}
	if bindMode == domain.BindDevice {
		device, err := normalizeRiskValue(domain.RiskBlockDevice, bindValue)
		if err != nil {
			return ErrInvalidRequest
		}
		if _, err := s.currentStore().FindActiveRiskBlock(ctx, productID, domain.RiskBlockDevice, s.riskHash(domain.RiskBlockDevice, device)); err == nil {
			s.recordRiskAlert(ctx, productID, "", "", "blocked_device", "critical", domain.RiskBlockDevice, device, "黑名单设备尝试访问授权接口")
			return ErrRiskBlockedDevice
		} else if !store.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func (s *Service) evaluateActivationRisk(ctx context.Context, productID string, lic domain.License, binding domain.Binding, clientIP, bindValue string) {
	if binding.BindMode == domain.BindDevice {
		count, err := s.currentStore().CountBindingsByHash(ctx, productID, binding.BindValueHash)
		if err == nil && count >= 3 {
			s.recordRiskAlert(ctx, productID, lic.ID, binding.ID, "device_multi_license", "high", domain.RiskBlockDevice, bindValue,
				fmt.Sprintf("同一设备已关联 %d 个授权", count))
		}
	}
	if ip, err := normalizeRiskValue(domain.RiskBlockIP, clientIP); err == nil {
		count, countErr := s.currentStore().CountRecentClientEventsByIP(ctx, productID, "license.activate", "success", clientIP, s.now().Add(-10*time.Minute))
		if countErr == nil && count >= 10 {
			s.recordRiskAlert(ctx, productID, lic.ID, binding.ID, "ip_activation_burst", "high", domain.RiskBlockIP, ip,
				fmt.Sprintf("同一 IP 在 10 分钟内完成 %d 次激活", count))
		}
	}
}

func (s *Service) evaluateFailedActivationRisk(ctx context.Context, productID, clientIP string) {
	ip, err := normalizeRiskValue(domain.RiskBlockIP, clientIP)
	if err != nil {
		return
	}
	count, err := s.currentStore().CountRecentClientEventsByIP(ctx, productID, "license.activate", "failed", clientIP, s.now().Add(-10*time.Minute))
	if err == nil && count >= 5 {
		s.recordRiskAlert(ctx, productID, "", "", "activation_failure_burst", "medium", domain.RiskBlockIP, ip,
			fmt.Sprintf("同一 IP 在 10 分钟内连续激活失败 %d 次", count))
	}
}

func (s *Service) recordRiskAlert(ctx context.Context, productID, licenseID, bindingID, alertType, severity, subjectKind, subjectValue, detail string) {
	value, err := normalizeRiskValue(subjectKind, subjectValue)
	if err != nil {
		return
	}
	now := s.now()
	_, _ = s.currentStore().UpsertRiskAlert(ctx, domain.RiskAlert{
		ID:              yncrypto.NewID("ralert"),
		ProductID:       productID,
		LicenseID:       licenseID,
		BindingID:       bindingID,
		AlertType:       alertType,
		Severity:        severity,
		Status:          domain.RiskAlertOpen,
		SubjectKind:     subjectKind,
		SubjectHash:     s.riskHash(subjectKind, value),
		SubjectMasked:   maskRiskValue(subjectKind, value),
		Detail:          detail,
		OccurrenceCount: 1,
		FirstSeenAt:     now,
		LastSeenAt:      now,
		CreatedAt:       now,
		UpdatedAt:       now,
	})
}

func (s *Service) auditClient(ctx context.Context, productID, licenseID, cardID, action, result, code, clientIP, userAgent string) error {
	return s.currentStore().InsertAudit(ctx, domain.AuditLog{
		ID:        yncrypto.NewID("audit"),
		ActorType: "client",
		ProductID: productID,
		LicenseID: licenseID,
		CardID:    cardID,
		Action:    action,
		ClientIP:  clientIP,
		UserAgent: userAgent,
		Result:    result,
		ErrorCode: code,
		CreatedAt: s.now(),
	})
}

func (s *Service) riskHash(kind, value string) string {
	return yncrypto.BindHash(s.cardPepper, "risk:"+kind, value)
}

func normalizeRiskValue(kind, raw string) (string, error) {
	value := strings.TrimSpace(raw)
	switch kind {
	case domain.RiskBlockIP:
		ip := net.ParseIP(value)
		if ip == nil {
			return "", ErrInvalidRequest
		}
		return ip.String(), nil
	case domain.RiskBlockDevice:
		if value == "" || len(value) > 256 {
			return "", ErrInvalidRequest
		}
		return value, nil
	default:
		return "", ErrInvalidRequest
	}
}

func maskRiskValue(kind, value string) string {
	if kind == domain.RiskBlockIP {
		ip := net.ParseIP(value)
		if ip4 := ip.To4(); ip4 != nil {
			return fmt.Sprintf("%d.%d.%d.*", ip4[0], ip4[1], ip4[2])
		}
		parts := strings.Split(value, ":")
		if len(parts) > 2 {
			return strings.Join(parts[:2], ":") + ":*"
		}
		return "*"
	}
	if len(value) <= 8 {
		return strings.Repeat("*", len(value))
	}
	return value[:4] + "..." + value[len(value)-4:]
}
