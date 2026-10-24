package service

import (
	"context"
	"encoding/hex"
	"net/url"
	"strings"
	"time"

	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

var ErrSecurityUse = forbidden("SECURITY_USE_DENIED", "application access requires administrator review")
var ErrSecurityUpdates = forbidden("SECURITY_UPDATES_DENIED", "updates are restricted by an administrator security decision")

type securityReader interface {
	SecurityPolicy(context.Context, string) (domain.SecurityPolicy, error)
	ActiveSecurityCase(context.Context, string) (domain.SecurityCase, error)
}

func validSecurityAction(a string) bool {
	return a == "observe" || a == "deny_updates" || a == "deny_use" || a == "block_page"
}
func (s *Service) SecurityPolicy(ctx context.Context, id string) (domain.SecurityPolicy, error) {
	if _, err := s.currentStore().GetProduct(ctx, id); err != nil {
		return domain.SecurityPolicy{}, ErrProductNotFound
	}
	return s.currentStore().SecurityPolicy(ctx, id)
}
func (s *Service) SetSecurityPolicy(ctx context.Context, id, actor string, p domain.SecurityPolicy) (domain.SecurityPolicy, error) {
	if !validSecurityAction(p.Action) || strings.TrimSpace(p.Message) == "" || len(p.Message) > 1000 || len(p.SupportURL) > 500 {
		return p, ErrInvalidRequest
	}
	if p.SupportURL != "" {
		u, err := url.Parse(p.SupportURL)
		if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
			return p, ErrInvalidRequest
		}
	}
	if _, err := s.currentStore().GetProduct(ctx, id); err != nil {
		return p, ErrProductNotFound
	}
	p.ProductID = id
	err := s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.LockProduct(ctx, id); err != nil {
			return err
		}
		if err := tx.SaveSecurityPolicy(ctx, p); err != nil {
			return err
		}
		return s.transactionAudit(ctx, tx, "admin", actor, id, "", "security.policy", p)
	})
	return p, err
}
func securityAction(ctx context.Context, q securityReader, lic domain.License) (string, domain.SecurityCase, domain.SecurityPolicy, error) {
	p, err := q.SecurityPolicy(ctx, lic.ProductID)
	if err != nil {
		return "", domain.SecurityCase{}, p, err
	}
	if !p.Enabled {
		return "observe", domain.SecurityCase{}, p, nil
	}
	c, err := q.ActiveSecurityCase(ctx, lic.ID)
	if store.IsNotFound(err) {
		return "observe", c, p, nil
	}
	if err != nil {
		return "", c, p, err
	}
	return c.Action, c, p, nil
}
func enforceSecurity(ctx context.Context, q securityReader, lic domain.License, updates bool) error {
	a, _, _, err := securityAction(ctx, q, lic)
	if err != nil {
		return err
	}
	if a == "deny_use" || a == "block_page" {
		return ErrSecurityUse
	}
	if updates && a == "deny_updates" {
		return ErrSecurityUpdates
	}
	return nil
}
func (s *Service) recordSecuritySignal(ctx context.Context, lic domain.License, signal, subject, evidence string) (domain.SecurityCase, error) {
	c := domain.SecurityCase{ID: yncrypto.NewID("case"), ProductID: lic.ProductID, LicenseID: lic.ID, LicenseNo: lic.LicenseNo, Signal: signal, Subject: subject, Evidence: evidence, Status: "open", Action: "observe", Occurrences: 1, FirstSeen: s.now(), LastSeen: s.now()}
	err := s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.LockLicense(ctx, lic.ID); err != nil {
			return err
		}
		var err error
		c, err = tx.RecordSecurityCase(ctx, c)
		if err != nil {
			return err
		}
		return s.transactionAudit(ctx, tx, "client", "", lic.ProductID, lic.ID, "security.signal", map[string]string{"case_id": c.ID, "signal": signal})
	})
	return c, err
}
func (s *Service) recordBindingMismatch(ctx context.Context, lic domain.License, mode, value string) error {
	if lic.ID == "" {
		return nil
	}
	p, err := s.currentStore().SecurityPolicy(ctx, lic.ProductID)
	if err != nil {
		return err
	}
	if !p.Enabled || !p.DetectMismatch {
		return nil
	}
	_, err = s.recordSecuritySignal(ctx, lic, "binding_mismatch", maskPublicQuery(mode, value), "服务端验证发现绑定不匹配；可能为配置错误，需人工核实。")
	return err
}

type IntegrityReportInput struct {
	VerifyInput
	ReleaseID      string `json:"release_id"`
	ObservedSHA256 string `json:"observed_sha256"`
}

func (s *Service) ReportIntegrity(ctx context.Context, in IntegrityReportInput) (domain.SecurityCase, error) {
	d, lic, err := s.securityDecision(ctx, in.VerifyInput)
	if err != nil {
		return domain.SecurityCase{}, err
	}
	if !d.CanUse {
		return domain.SecurityCase{}, ErrSecurityUse
	}
	p, err := s.currentStore().SecurityPolicy(ctx, lic.ProductID)
	if err != nil {
		return domain.SecurityCase{}, err
	}
	if !p.Enabled || !p.AcceptReports {
		return domain.SecurityCase{}, forbidden("SECURITY_REPORTS_DISABLED", "client integrity reporting is disabled")
	}
	in.ObservedSHA256 = strings.ToLower(in.ObservedSHA256)
	raw, err := hex.DecodeString(in.ObservedSHA256)
	if err != nil || len(raw) != 32 {
		return domain.SecurityCase{}, ErrInvalidRequest
	}
	r, err := s.currentStore().GetRelease(ctx, in.ReleaseID)
	if err != nil || r.ProductID != lic.ProductID || r.Status != "published" {
		return domain.SecurityCase{}, ErrReleaseNotFound
	}
	if in.ObservedSHA256 == r.SHA256 {
		return domain.SecurityCase{}, badRequest("INTEGRITY_MATCHES", "reported package hash matches the published release")
	}
	return s.recordSecuritySignal(ctx, lic, "integrity_mismatch", r.Version, "客户端报告（未经独立证实）：版本 "+r.Version+"；预期 SHA256 "+r.SHA256+"；报告 SHA256 "+in.ObservedSHA256)
}

type SecurityDecision struct {
	Action     string    `json:"action"`
	CaseID     string    `json:"case_id"`
	CanUse     bool      `json:"can_use"`
	CanUpdate  bool      `json:"can_update"`
	Message    string    `json:"message"`
	SupportURL string    `json:"support_url"`
	ServerTime time.Time `json:"server_time"`
}

func (s *Service) securityDecision(ctx context.Context, in VerifyInput) (SecurityDecision, domain.License, error) {
	var d SecurityDecision
	var lic domain.License
	p, err := s.productByAppKey(ctx, in.AppKey)
	if err != nil {
		return d, lic, err
	}
	mode, value, hash, err := s.normalizeBinding(p, in.BindMode, in.BindValue)
	if err != nil {
		return d, lic, err
	}
	if err = s.checkRiskAccess(ctx, p.ID, in.ClientIP, mode, value); err != nil {
		return d, lic, err
	}
	lic, err = s.currentStore().GetLicenseByNo(ctx, in.LicenseNo)
	if err != nil || lic.ProductID != p.ID {
		return d, lic, ErrLicenseNotFound
	}
	bindings, err := s.currentStore().GetBindings(ctx, lic.ID)
	if err != nil {
		return d, lic, err
	}
	found := false
	for _, b := range bindings {
		found = found || b.Status == domain.BindingActive && b.BindMode == mode && b.BindValueHash == hash
	}
	if !found {
		return d, lic, ErrBindingMismatch
	}
	a, c, policy, err := securityAction(ctx, s.currentStore(), lic)
	if err != nil {
		return d, lic, err
	}
	d = SecurityDecision{Action: a, CaseID: c.ID, CanUse: s.ensureUsable(p, lic, s.now()) == nil, CanUpdate: licenseCanUpdate(lic, s.now()), Message: policy.Message, SupportURL: policy.SupportURL, ServerTime: s.now()}
	if !d.CanUse {
		d.CanUpdate = false
		d.Action = "authorization_unavailable"
		d.CaseID = ""
		d.Message = "当前授权不可用，请核对授权状态或续费。"
		return d, lic, nil
	}
	if a == "deny_use" || a == "block_page" {
		d.CanUse = false
		d.CanUpdate = false
	} else if a == "deny_updates" {
		d.CanUpdate = false
	}
	return d, lic, nil
}
func (s *Service) SecurityDecision(ctx context.Context, in VerifyInput) (SecurityDecision, error) {
	d, _, err := s.securityDecision(ctx, in)
	return d, err
}
func (s *Service) SecurityCases(ctx context.Context, user string) ([]domain.SecurityCase, error) {
	items, err := s.currentStore().SecurityCases(ctx, user)
	if user != "" {
		for i := range items {
			items[i].Evidence = ""
			items[i].Reviewer = ""
			items[i].ReviewReason = ""
		}
	}
	return items, err
}

type ReviewSecurityInput struct {
	Status string `json:"status"`
	Action string `json:"action"`
	Reason string `json:"reason"`
}

func (s *Service) ReviewSecurityCase(ctx context.Context, id, actor string, in ReviewSecurityInput) error {
	if (in.Status != "confirmed" && in.Status != "resolved") || strings.TrimSpace(in.Reason) == "" || len(in.Reason) > 1000 {
		return ErrInvalidRequest
	}
	c, err := s.currentStore().SecurityCase(ctx, id)
	if err != nil {
		return notFound("SECURITY_CASE_NOT_FOUND", "case not found")
	}
	return s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.LockLicense(ctx, c.LicenseID); err != nil {
			return err
		}
		p, err := tx.SecurityPolicy(ctx, c.ProductID)
		if err != nil {
			return err
		}
		if in.Action == "" {
			in.Action = p.Action
		}
		if in.Status == "resolved" {
			in.Action = "observe"
		}
		if !validSecurityAction(in.Action) {
			return ErrInvalidRequest
		}
		if err := tx.ReviewSecurityCase(ctx, id, in.Status, in.Action, in.Reason, actor); err != nil {
			return err
		}
		return s.transactionAudit(ctx, tx, "admin", actor, c.ProductID, c.LicenseID, "security.review", map[string]any{"case_id": id, "review": in})
	})
}
func (s *Service) AppealSecurityCase(ctx context.Context, user, id, reason string) (domain.SecurityAppeal, error) {
	var a domain.SecurityAppeal
	if strings.TrimSpace(reason) == "" || len(reason) > 1000 {
		return a, ErrInvalidRequest
	}
	c, err := s.currentStore().SecurityCase(ctx, id)
	if err != nil {
		return a, ErrLicenseNotFound
	}
	err = s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.LockLicense(ctx, c.LicenseID); err != nil {
			return err
		}
		owner, err := tx.Owner(ctx, c.LicenseID)
		if err != nil {
			return err
		}
		if owner != user {
			return ErrLicenseNotFound
		}
		fresh, err := tx.SecurityCase(ctx, id)
		if err != nil {
			return err
		}
		if fresh.Status == "resolved" {
			return badRequest("SECURITY_CASE_RESOLVED", "case is already resolved")
		}
		a = domain.SecurityAppeal{ID: yncrypto.NewID("appeal"), CaseID: id, UserID: user, Reason: reason, Status: "open", CreatedAt: s.now()}
		a, err = tx.Appeal(ctx, a)
		if err != nil {
			return err
		}
		return s.transactionAudit(ctx, tx, "customer", user, c.ProductID, c.LicenseID, "security.appeal", map[string]string{"case_id": id, "appeal_id": a.ID})
	})
	return a, err
}
func (s *Service) SecurityAppeals(ctx context.Context, user string) ([]domain.SecurityAppeal, error) {
	return s.currentStore().SecurityAppeals(ctx, user)
}
