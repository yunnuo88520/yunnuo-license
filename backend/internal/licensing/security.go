package licensing

import (
	"context"
	"encoding/hex"
	"net/url"
	"strings"
)

type BlockPolicy struct {
	Enabled      bool   `json:"enabled"`
	Announcement string `json:"announcement"`
	SupportURL   string `json:"support_url"`
}

func normalizeBlockPolicy(p *BlockPolicy) error {
	if len(p.Announcement) > 3000 || len(p.SupportURL) > 1000 {
		return invalid("拦截公告或联系链接过长")
	}
	if p.SupportURL != "" {
		u, e := url.Parse(p.SupportURL)
		if e != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
			return invalid("联系链接需要有效的 HTTP(S) 地址")
		}
	}
	return nil
}
func (s *Service) PublicBlockPolicy(ctx context.Context, id string) (map[string]any, error) {
	app, e := s.registry.GetApplication(ctx, id)
	if e != nil {
		return nil, notFound()
	}
	return map[string]any{"application_name": app.Name, "policy": app.BlockPolicy}, nil
}

type IntegrityReportInput struct {
	AppID          string `json:"app_id"`
	ActivationID   string `json:"activation_id"`
	Signal         string `json:"signal"`
	EvidenceSHA256 string `json:"evidence_sha256"`
	Summary        string `json:"summary"`
}

// Client integrity signals are evidence, not an automatic accusation. Only
// an authenticated operator can restrict usage or updates after review.
func (s *Service) ReportIntegrity(ctx context.Context, in IntegrityReportInput, req SignedRequest) (map[string]string, error) {
	if in.Signal != "binary_modified" && in.Signal != "unexpected_signature" && in.Signal != "copied_installation" {
		return nil, invalid("完整性信号类型无效")
	}
	if len(in.Summary) > 1000 || len(in.EvidenceSHA256) != 64 {
		return nil, invalid("请提交证据摘要，不要上传源文件或原始设备标识")
	}
	if _, e := hex.DecodeString(in.EvidenceSHA256); e != nil {
		return nil, invalid("证据摘要需要 SHA-256")
	}
	// Resolve the public installation key in an application-bound read before
	// authenticating the proof. Knowing an activation ID is never sufficient.
	scoped, _, repo, e := s.runtimeContext(ctx, in.AppID, Actor{Kind: "installation_lookup", ID: in.ActivationID})
	if e != nil {
		return nil, e
	}
	var a Activation
	e = repo.View(scoped, func(v View) error { var e error; a, e = v.Activations().Get(scoped, in.ActivationID); return e })
	if e != nil {
		return nil, notFound()
	}
	scoped, _, repo, e = s.authenticateInstallation(ctx, in.AppID, a.PublicKey, req)
	if e != nil {
		return nil, e
	}
	eventID := ""
	e = repo.Update(scoped, func(tx Transaction) error {
		current, e := tx.Activations().Get(scoped, in.ActivationID)
		if e != nil || current.PublicKey != a.PublicKey {
			return notFound()
		}
		event := s.event(scoped, "integrity_reported", current.LicenseID, "pending", in.Signal, map[string]any{"activation_id": a.ID, "evidence_sha256": in.EvidenceSHA256, "summary": in.Summary})
		eventID = event.ID
		return tx.AppendAudit(scoped, event, s.auditSigner)
	})
	return map[string]string{"event_id": eventID, "status": "pending"}, e
}

type SecurityDecisionInput struct {
	Action           string `json:"action"`
	Reason           string `json:"reason"`
	ReferenceEventID string `json:"reference_event_id"`
}

func (s *Service) ReviewLicenseSecurity(ctx context.Context, id, licenseID string, in SecurityDecisionInput) error {
	scoped, _, repo, e := s.applicationContext(ctx, id, true)
	if e != nil {
		return e
	}
	p, _ := PrincipalFrom(ctx)
	if p.Actor.Kind != "admin" && p.Actor.Kind != "developer" {
		return forbidden("ROLE_FORBIDDEN", "请由管理员或所属开发者核实处置")
	}
	if strings.TrimSpace(in.Reason) == "" || len(in.Reason) > 1000 {
		return invalid("请填写人工核实原因")
	}
	if in.Action != "allow_updates" && in.Action != "deny_updates" && in.Action != "observe" {
		return invalid("更新处置无效；需要停用产品时请另行暂停或吊销许可证")
	}
	if in.ReferenceEventID != "" {
		e = repo.View(scoped, func(v View) error {
			event, e := v.Audit().Get(scoped, in.ReferenceEventID)
			if e != nil || event.EntityID != licenseID {
				return invalid("处置引用必须属于当前应用与许可证")
			}
			return nil
		})
		if e != nil {
			return e
		}
	}
	return repo.Update(scoped, func(tx Transaction) error {
		l, e := tx.Licenses().Get(scoped, licenseID)
		if e != nil {
			return notFound()
		}
		if in.Action != "observe" {
			l.UpdatesBlocked = in.Action == "deny_updates"
			l.UpdatedAt = s.now()
			if e = tx.Licenses().Save(scoped, l); e != nil {
				return e
			}
		}
		return s.append(scoped, tx, "security_reviewed", licenseID, map[string]any{"action": in.Action, "reason": in.Reason, "reference_event_id": in.ReferenceEventID})
	})
}

func (s *Service) SubmitSecurityAppeal(ctx context.Context, id, licenseID, reason string) (map[string]string, error) {
	scoped, _, repo, e := s.licenseOwnerContext(ctx, id, licenseID, true)
	if e != nil {
		return nil, e
	}
	p, _ := PrincipalFrom(ctx)
	if p.Actor.Kind != "customer" {
		return nil, forbidden("ROLE_FORBIDDEN", "请使用授权所属客户账号提交申诉")
	}
	if len(strings.TrimSpace(reason)) < 6 || len(reason) > 3000 {
		return nil, invalid("请填写 6–3000 字节的申诉说明")
	}
	eventID := ""
	e = repo.Update(scoped, func(tx Transaction) error {
		l, e := tx.Licenses().Get(scoped, licenseID)
		if e != nil || l.OwnerUserID != p.Actor.ID {
			return notFound()
		}
		event := s.event(scoped, "security_appeal", licenseID, "pending", "", map[string]any{"reason": reason})
		eventID = event.ID
		return tx.AppendAudit(scoped, event, s.auditSigner)
	})
	return map[string]string{"event_id": eventID, "status": "pending"}, e
}
