package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
	"regexp"
	"time"
)

var instanceIDPattern = regexp.MustCompile(`^[a-z0-9_-]{16,64}$`)

func (s *Service) instanceHash(kind, value string) string {
	h := hmac.New(sha256.New, s.cardPepper)
	h.Write([]byte(kind + "\x00" + value))
	return hex.EncodeToString(h.Sum(nil))
}
func validateInstance(p domain.Product, in domain.ClientInstance) error {
	t := domain.LegacyTerms(p)
	if !t.RequireInstance && in.InstanceID == "" && in.InstanceSecret == "" {
		return nil
	}
	secret, e := hex.DecodeString(in.InstanceSecret)
	if !instanceIDPattern.MatchString(in.InstanceID) || e != nil || len(secret) != 32 || len(in.DeviceID) > 256 || t.MaxDevices > 0 && in.DeviceID == "" {
		return badRequest("INSTANCE_REQUIRED", "需要本机持久化的实例 ID、32 字节实例凭证及套餐要求的设备标识")
	}
	return nil
}
func (s *Service) registerInstallationTx(ctx context.Context, tx *store.Tx, p domain.Product, l domain.License, b domain.Binding, in ActivateInput) error {
	// A signed-in customer may claim a card first; installing with that same card
	// subsequently registers the actual private instance credential.
	if in.OwnerUserID != "" && in.InstanceID == "" && in.InstanceSecret == "" {
		return nil
	}
	if e := validateInstance(p, in.ClientInstance); e != nil {
		return e
	}
	if in.InstanceID == "" {
		return nil
	}
	if p.BindMode == "instance" && in.BindValue != in.InstanceID {
		return ErrBindingMismatch
	}
	if p.BindMode == "composite" {
		var m map[string]string
		_ = json.Unmarshal([]byte(in.BindValue), &m)
		if id, ok := m["instance"]; ok && id != in.InstanceID {
			return ErrBindingMismatch
		}
	}
	existing, e := tx.Installation(ctx, l.ID, in.InstanceID)
	if e == nil {
		if existing.Status != "active" {
			return forbidden("INSTANCE_DISABLED", "此安装实例已停用")
		}
		if subtle.ConstantTimeCompare([]byte(existing.SecretHash), []byte(s.instanceHash("instance", in.InstanceSecret))) != 1 {
			return forbidden("INSTANCE_INVALID", "安装实例凭证不正确")
		}
		if existing.BindingID != b.ID {
			return ErrBindingMismatch
		}
		return tx.TouchInstallation(ctx, l.ID, in.InstanceID, s.now())
	}
	if !store.IsNotFound(e) {
		return e
	}
	device := in.DeviceID
	if device == "" {
		device = in.InstanceID
	}
	hash := s.instanceHash("device", device)
	count, devices, exists, e := tx.InstallationCounts(ctx, l.ID, hash)
	if e != nil {
		return e
	}
	t := domain.LegacyTerms(p)
	if count >= t.MaxDeployments {
		return forbidden("DEPLOYMENT_LIMIT_EXCEEDED", "已达到购买的安装实例数量")
	}
	if t.MaxDevices > 0 && exists == 0 && devices >= t.MaxDevices {
		return ErrDeviceLimitExceeded
	}
	v := domain.Installation{LicenseID: l.ID, ID: in.InstanceID, BindingID: b.ID, SecretHash: s.instanceHash("instance", in.InstanceSecret), DeviceHash: hash, Label: in.DeviceName, Status: "active", LastSeenAt: s.now(), CreatedAt: s.now()}
	if len(v.Label) > 160 {
		return ErrInvalidRequest
	}
	if e = tx.CreateInstallation(ctx, v); e != nil {
		return e
	}
	return s.transactionAudit(ctx, tx, "client", in.InstanceID, p.ID, l.ID, "installation.register", map[string]string{"instance_id": in.InstanceID})
}
func (s *Service) checkInstallationTx(ctx context.Context, tx *store.Tx, p domain.Product, l domain.License, b domain.Binding, in domain.ClientInstance, requireSeat bool) error {
	if e := validateInstance(p, in); e != nil {
		return e
	}
	if in.InstanceID == "" {
		return nil
	}
	v, e := tx.Installation(ctx, l.ID, in.InstanceID)
	if e != nil || v.Status != "active" || subtle.ConstantTimeCompare([]byte(v.SecretHash), []byte(s.instanceHash("instance", in.InstanceSecret))) != 1 {
		return forbidden("INSTANCE_INVALID", "安装实例不存在、已停用或凭证不正确")
	}
	if v.BindingID != b.ID {
		return ErrBindingMismatch
	}
	if domain.LegacyTerms(p).MaxDevices > 0 && v.DeviceHash != s.instanceHash("device", in.DeviceID) {
		return ErrBindingMismatch
	}
	if requireSeat && domain.LegacyTerms(p).MaxConcurrentSeats > 0 {
		seats, e := tx.Seats(ctx, l.ID, s.now())
		if e != nil {
			return e
		}
		valid := false
		for _, seat := range seats {
			if seat.ID == in.SeatID && seat.InstanceID == in.InstanceID && seat.SessionID == in.SessionID {
				valid = true
				break
			}
		}
		if !valid {
			return forbidden("SEAT_REQUIRED", "请先获取有效的并发席位")
		}
	}
	return tx.TouchInstallation(ctx, l.ID, in.InstanceID, s.now())
}

type SeatInput struct {
	VerifyInput
	SessionID  string `json:"session_id"`
	BorrowDays int    `json:"borrow_days"`
}

func (s *Service) AcquireSeat(ctx context.Context, in SeatInput) (domain.SeatLease, error) {
	var result domain.SeatLease
	var granted domain.License
	if !instanceIDPattern.MatchString(in.SessionID) || in.BorrowDays < 0 {
		return result, ErrInvalidRequest
	}
	in.skipSeat = true
	if _, e := s.Verify(ctx, in.VerifyInput); e != nil {
		return result, e
	}
	p, e := s.productByAppKey(ctx, in.AppKey)
	if e != nil {
		return result, e
	}
	e = s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		l, e := tx.GetLicenseByNo(ctx, in.LicenseNo)
		if e != nil || l.ProductID != p.ID {
			return ErrLicenseNotFound
		}
		if e = tx.LockLicense(ctx, l.ID); e != nil {
			return e
		}
		l, e = tx.GetLicenseByID(ctx, l.ID)
		if e != nil {
			return e
		}
		granted = l
		p = effectiveProduct(p, l.AuthorizationTerms)
		t := domain.LegacyTerms(p)
		fresh, e := tx.GetProduct(ctx, p.ID)
		if e != nil || fresh.Status != domain.ProductActive {
			return ErrProductDisabled
		}
		if e = s.ensureUsable(p, l, s.now()); e != nil {
			return e
		}
		if e = enforceSecurity(ctx, tx, l, false); e != nil {
			return e
		}
		mode, value, hash, e := s.normalizeBinding(p, in.BindMode, bindingInput(in.BindValue, in.Bindings))
		if e != nil {
			return e
		}
		b, e := s.authorizedBinding(ctx, tx, p, l.ID, mode, value, hash)
		if e != nil {
			return ErrBindingMismatch
		}
		if e = s.checkInstallationTx(ctx, tx, p, l, b, in.ClientInstance, false); e != nil {
			return e
		}
		if t.MaxConcurrentSeats < 1 || in.InstanceID == "" {
			return badRequest("SEATS_DISABLED", "当前套餐未启用浮动席位")
		}
		if in.BorrowDays > t.OfflineGraceDays {
			return forbidden("OFFLINE_BORROW_DENIED", "离线借用超过套餐允许期限")
		}
		seats, e := tx.Seats(ctx, l.ID, s.now())
		if e != nil {
			return e
		}
		for _, v := range seats {
			if v.InstanceID == in.InstanceID && v.SessionID == in.SessionID {
				result = v
				break
			}
		}
		if result.ID != "" && result.Borrowed {
			return nil
		}
		if result.ID == "" && len(seats) >= t.MaxConcurrentSeats {
			return forbidden("SEAT_LIMIT_EXCEEDED", "并发席位已全部使用，请等待释放")
		}
		if result.ID == "" {
			result = domain.SeatLease{ID: yncrypto.NewID("seat"), LicenseID: l.ID, InstanceID: in.InstanceID, SessionID: in.SessionID}
		}
		until := s.now().Add(time.Duration(t.SeatLeaseSeconds) * time.Second)
		if in.BorrowDays > 0 {
			until = s.now().Add(time.Duration(in.BorrowDays) * 24 * time.Hour)
			result.Borrowed = true
		}
		if grace := usageGraceUntil(p, l); grace != nil && grace.Before(until) {
			until = *grace
		}
		result.ExpiresAt = until
		if e = tx.SaveSeat(ctx, result); e != nil {
			return e
		}
		return s.transactionAudit(ctx, tx, "client", in.InstanceID, p.ID, l.ID, "seat.acquire", map[string]any{"seat_id": result.ID, "borrowed": result.Borrowed, "expires_at": until})
	})
	if e != nil {
		return result, e
	}
	private, e := yncrypto.DecryptString(s.dataKey, p.PrivateKeyEncrypted)
	if e != nil {
		return result, e
	}
	mode, value, _, e := s.normalizeBinding(p, in.BindMode, bindingInput(in.BindValue, in.Bindings))
	if e != nil {
		return result, e
	}
	offlineUntil := result.ExpiresAt
	if domain.LegacyTerms(p).OfflineGraceDays == 0 {
		offlineUntil = s.now()
	}
	result.Token, e = yncrypto.SignJSON(private, map[string]any{"type": "offline_cache", "purpose": "seat_lease", "product_code": p.Code, "license_no": in.LicenseNo, "instance_id": in.InstanceID, "session_id": result.SessionID, "seat_id": result.ID, "issued_at": s.now(), "offline_until": offlineUntil, "expires_at": result.ExpiresAt, "borrowed": result.Borrowed, "bind_mode": mode, "bind_digest": yncrypto.BindDigest(mode, value), "can_use": true, "can_update": false, "key_version": p.KeyVersion, "authorization_terms": p.AuthorizationTerms, "license_mode": p.LicenseMode, "license_expired_at": usageGraceUntil(p, granted), "usage_expired_at": granted.ExpiredAt, "updates_expired_at": granted.UpdatesExpiredAt})
	return result, e
}
func (s *Service) ReleaseSeat(ctx context.Context, in VerifyInput) error {
	in.skipSeat = true
	if _, e := s.Verify(ctx, in); e != nil {
		return e
	}
	return s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		l, e := tx.GetLicenseByNo(ctx, in.LicenseNo)
		if e != nil {
			return e
		}
		if e = tx.ReleaseSeat(ctx, l.ID, in.InstanceID, in.SeatID, in.SessionID); e != nil {
			return e
		}
		return s.transactionAudit(ctx, tx, "client", in.InstanceID, l.ProductID, l.ID, "seat.release", map[string]string{"seat_id": in.SeatID})
	})
}
func (s *Service) CreateLicenseClaim(ctx context.Context, in VerifyInput) (map[string]any, error) {
	in.skipSeat = true
	if _, e := s.Verify(ctx, in); e != nil {
		return nil, e
	}
	if in.InstanceID == "" {
		return nil, badRequest("INSTANCE_REQUIRED", "请从已激活产品生成关联凭证")
	}
	token, e := yncrypto.ApplicationSecret()
	if e != nil {
		return nil, e
	}
	until := s.now().Add(10 * time.Minute)
	e = s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		l, e := tx.GetLicenseByNo(ctx, in.LicenseNo)
		if e != nil {
			return e
		}
		if e = tx.LockLicense(ctx, l.ID); e != nil {
			return e
		}
		l, e = tx.GetLicenseByID(ctx, l.ID)
		if e != nil {
			return e
		}
		if l.OwnerUserID != "" {
			return ErrOwnerRequired
		}
		p, e := tx.GetProduct(ctx, l.ProductID)
		if e != nil {
			return e
		}
		p = effectiveProduct(p, l.AuthorizationTerms)
		if e = s.ensureUsable(p, l, s.now()); e != nil {
			return e
		}
		if e = enforceSecurity(ctx, tx, l, false); e != nil {
			return e
		}
		mode, value, hash, e := s.normalizeBinding(p, in.BindMode, bindingInput(in.BindValue, in.Bindings))
		if e != nil {
			return e
		}
		binding, e := s.authorizedBinding(ctx, tx, p, l.ID, mode, value, hash)
		if e != nil {
			return ErrBindingMismatch
		}
		if e = s.checkInstallationTx(ctx, tx, p, l, binding, in.ClientInstance, false); e != nil {
			return e
		}
		return tx.CreateClaimTicket(ctx, s.instanceHash("claim", token), l.ID, until)
	})
	return map[string]any{"claim_token": token, "expires_at": until}, e
}
func (s *Service) RedeemLicenseClaim(ctx context.Context, user, token string) (map[string]string, error) {
	if len(token) != 32 {
		return nil, ErrOwnerRequired
	}
	no := ""
	e := s.currentStore().WithTx(ctx, func(tx *store.Tx) error {
		id, claimed, until, e := tx.ClaimTicket(ctx, s.instanceHash("claim", token))
		if e != nil || !s.now().Before(until) || claimed != "" && claimed != user {
			return ErrOwnerRequired
		}
		if e = tx.LockLicense(ctx, id); e != nil {
			return e
		}
		l, e := tx.GetLicenseByID(ctx, id)
		if e != nil {
			return e
		}
		if l.OwnerUserID != "" && l.OwnerUserID != user {
			return ErrOwnerRequired
		}
		if e = tx.SetOwner(ctx, id, user, s.now()); e != nil {
			return e
		}
		if e = tx.ConsumeClaimTicket(ctx, s.instanceHash("claim", token), user); e != nil {
			return e
		}
		no = l.LicenseNo
		return s.transactionAudit(ctx, tx, "customer", user, l.ProductID, id, "license.claim_ticket", nil)
	})
	return map[string]string{"license_no": no}, e
}
