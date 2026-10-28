package licensing

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
)

type SaleInput struct {
	CardDigests     []string   `json:"card_sha256,omitempty"`
	PlanID          string     `json:"plan_id"`
	CustomerRef     string     `json:"customer_ref"`
	OwnerUserID     string     `json:"owner_user_id"`
	Quantity        int        `json:"quantity"`
	AmountCents     int64      `json:"amount_cents"`
	DistributeCards bool       `json:"distribute_cards"`
	Purpose         string     `json:"purpose"`
	RedeemUntil     *time.Time `json:"redeem_until"`
	Terms           *Terms     `json:"terms,omitempty"`
	Reason          string     `json:"reason"`
}
type CardDelivery struct {
	CardID    string `json:"card_id"`
	LicenseID string `json:"license_id"`
	Code      string `json:"code"`
}
type SaleResult struct {
	Order    SalesOrder     `json:"order"`
	Cards    []CardDelivery `json:"cards"`
	Replayed bool           `json:"replayed"`
}

func validateSale(in SaleInput) error {
	if in.PlanID == "" || in.Quantity < 1 || in.Quantity > 1000 || len(in.CustomerRef) > 190 || len(in.OwnerUserID) > 96 || in.AmountCents < 0 || in.AmountCents > 1_000_000_000_000 {
		return invalid("套餐、数量、客户或金额无效")
	}
	if len(in.CardDigests) > 0 {
		if !in.DistributeCards || len(in.CardDigests) != in.Quantity {
			return invalid("商城分发的卡密摘要数量必须等于发货数量")
		}
		seen := map[string]bool{}
		for _, digest := range in.CardDigests {
			if !fingerprintPattern.MatchString(digest) || seen[digest] {
				return invalid("卡密摘要需要去重的小写 SHA-256")
			}
			seen[digest] = true
		}
	}
	if in.Purpose != "" && in.Purpose != "activate" && in.Purpose != "renew_usage" && in.Purpose != "renew_updates" {
		return invalid("卡密用途无效")
	}
	if in.Purpose != "" && in.Purpose != "activate" && !in.DistributeCards {
		return invalid("续费销售需要分发续费卡密")
	}
	return nil
}
func (s *Service) CreateSale(ctx context.Context, appID string, in SaleInput, confirmed bool) (SaleResult, error) {
	scoped, app, repo, e := s.applicationContext(ctx, appID, true)
	if e != nil {
		return SaleResult{}, e
	}
	if e = validateSale(in); e != nil {
		return SaleResult{}, e
	}
	if confirmed && strings.TrimSpace(in.Reason) == "" {
		return SaleResult{}, invalid("人工确认收款或发卡需要填写原因")
	}
	var out SaleResult
	e = repo.Update(scoped, func(tx Transaction) error {
		current, e := tx.Application(scoped)
		if e != nil {
			return e
		}
		app = current
		plan, e := tx.Plans().Get(scoped, in.PlanID)
		if e != nil {
			return notFound()
		}
		if !plan.Enabled {
			return forbidden("PLAN_DISABLED", "套餐已停用")
		}
		terms := plan.Terms
		if in.Terms != nil {
			terms = *in.Terms
		}
		terms, e = NormalizeTerms(app, terms)
		if e != nil {
			return e
		}
		order := SalesOrder{Record: s.record(appID, "order"), PlanID: plan.ID, CustomerRef: in.CustomerRef, OwnerUserID: in.OwnerUserID, Quantity: in.Quantity, AmountCents: in.AmountCents, Currency: plan.Currency, Status: "pending", Terms: terms, DistributeCards: in.DistributeCards, CardDigests: in.CardDigests, Purpose: in.Purpose, RedeemUntil: in.RedeemUntil, Reason: in.Reason}
		if e = tx.Orders().Insert(scoped, order); e != nil {
			return e
		}
		out.Order = order
		if confirmed {
			out, e = s.issueOrder(scoped, tx, app, order, in.Purpose, in.RedeemUntil)
			if e != nil {
				return e
			}
		}
		return s.append(scoped, tx, "sale_created", order.ID, map[string]any{"quantity": in.Quantity, "confirmed": confirmed, "distribute_cards": in.DistributeCards, "reason": in.Reason})
	})
	return out, e
}
func (s *Service) issueOrder(ctx context.Context, tx Transaction, app Application, order SalesOrder, purpose string, redeemUntil *time.Time) (SaleResult, error) {
	if order.Status != "pending" {
		return SaleResult{}, conflict("ORDER_ALREADY_CONFIRMED", "订单已处理，卡密原文无法再次导出")
	}
	if app.Status != "active" {
		return SaleResult{}, forbidden("APPLICATION_DISABLED", "应用已停用")
	}
	if _, e := NormalizeTerms(app, order.Terms); e != nil {
		return SaleResult{}, e
	}
	if purpose == "" {
		purpose = "activate"
	}
	now := s.now()
	if redeemUntil != nil && !now.Before(*redeemUntil) {
		return SaleResult{}, invalid("卡密兑换截止时间必须晚于当前时间")
	}
	out := SaleResult{Cards: []CardDelivery{}}
	batch := yncrypto.NewID("batch")
	for i := 0; i < order.Quantity; i++ {
		l := License{Record: s.record(app.ApplicationID, "license"), PlanID: order.PlanID, CustomerRef: order.CustomerRef, OwnerUserID: order.OwnerUserID, OrderID: order.ID, Status: "active", Terms: order.Terms, BaseDurationDays: order.Terms.DurationDays, BaseUpdateDays: order.Terms.UpdateDurationDays, RenewalOnly: purpose != "activate"}
		if !l.RenewalOnly && l.Terms.StartsOn == "issue" {
			startLicense(&l, now)
		}
		if e := tx.Licenses().Insert(ctx, l); e != nil {
			return SaleResult{}, e
		}
		order.LicenseIDs = append(order.LicenseIDs, l.ID)
		if order.DistributeCards {
			raw, lastFour, source, keyHash := "", "", "platform", ""
			var e error
			if len(order.CardDigests) > 0 {
				source = "sales_system"
				keyHash = s.hash("card", app.ApplicationID, order.CardDigests[i])
				existing, e := tx.Cards().Count(ctx, Filter{KeyHash: keyHash})
				if e != nil {
					return SaleResult{}, e
				}
				if existing > 0 {
					return SaleResult{}, conflict("CARD_DIGEST_EXISTS", "商城卡密摘要已用于其他发货，请核对幂等事件 ID")
				}
			} else {
				raw, e = randomHex(20)
				if e != nil {
					return SaleResult{}, e
				}
				raw = "YN2-" + strings.ToUpper(raw)
				lastFour = raw[len(raw)-4:]
				keyHash = s.cardHash(app.ApplicationID, raw)
			}
			card := CardKey{Record: s.record(app.ApplicationID, "card"), PlanID: order.PlanID, BatchID: batch, KeyHash: keyHash, LastFour: lastFour, Source: source, Purpose: purpose, Status: "unused", BoundLicenseID: l.ID, RedeemUntil: redeemUntil}
			if e = tx.Cards().Insert(ctx, card); e != nil {
				return SaleResult{}, e
			}
			out.Cards = append(out.Cards, CardDelivery{card.ID, l.ID, raw})
		}
	}
	order.Status = "confirmed"
	order.UpdatedAt = now
	if e := tx.Orders().Save(ctx, order); e != nil {
		return SaleResult{}, e
	}
	out.Order = order
	return out, nil
}
func startLicense(l *License, now time.Time) {
	if l.ActivatedAt != nil {
		return
	}
	l.ActivatedAt = &now
	if !l.Terms.Perpetual {
		v := now.AddDate(0, 0, l.BaseDurationDays)
		l.BaseExpiresAt = &v
	}
	baseUpdate := now.AddDate(0, 0, l.BaseUpdateDays)
	l.BaseUpdatesUntil = &baseUpdate

	if !l.Terms.Perpetual {
		v := now.Add(time.Duration(l.Terms.DurationDays) * 24 * time.Hour)
		l.ExpiresAt = &v
	}
	// Zero update days means no update entitlement, never an implicit lifetime grant.
	v := now.Add(time.Duration(l.Terms.UpdateDurationDays) * 24 * time.Hour)
	l.UpdatesUntil = &v
	l.UpdatedAt = now
}
func normalizeCard(raw string) string { return strings.ToUpper(strings.TrimSpace(raw)) }
func (s *Service) ConfirmSale(ctx context.Context, appID, orderID, reason string) (SaleResult, error) {
	scoped, _, repo, e := s.applicationContext(ctx, appID, true)
	if e != nil {
		return SaleResult{}, e
	}
	if strings.TrimSpace(reason) == "" {
		return SaleResult{}, invalid("请填写确认收款原因")
	}
	var out SaleResult
	e = repo.Update(scoped, func(tx Transaction) error {
		app, e := tx.Application(scoped)
		if e != nil {
			return e
		}
		order, e := tx.Orders().Get(scoped, orderID)
		if e != nil {
			return notFound()
		}
		order.Reason = reason
		out, e = s.issueOrder(scoped, tx, app, order, order.Purpose, order.RedeemUntil)
		if e != nil {
			return e
		}
		return s.append(scoped, tx, "sale_confirmed", orderID, map[string]any{"reason": reason})
	})
	return out, e
}
func (s *Service) ListSales(ctx context.Context, appID string) ([]SalesOrder, error) {
	scoped, _, repo, e := s.applicationContext(ctx, appID, false)
	if e != nil {
		return nil, e
	}
	var out []SalesOrder
	e = repo.View(scoped, func(v View) error { var e error; out, e = v.Orders().Find(scoped, Filter{Limit: 1000}); return e })
	return out, e
}
func (s *Service) ListCards(ctx context.Context, appID string) ([]CardKey, error) {
	scoped, _, repo, e := s.applicationContext(ctx, appID, false)
	if e != nil {
		return nil, e
	}
	var out []CardKey
	e = repo.View(scoped, func(v View) error { var e error; out, e = v.Cards().Find(scoped, Filter{Limit: 1000}); return e })
	return out, e
}
func (s *Service) RevokeCard(ctx context.Context, appID, cardID, reason string) error {
	scoped, _, repo, e := s.applicationContext(ctx, appID, true)
	if e != nil {
		return e
	}
	if strings.TrimSpace(reason) == "" {
		return invalid("请填写吊销原因")
	}
	return repo.Update(scoped, func(tx Transaction) error {
		card, e := tx.Cards().Get(scoped, cardID)
		if e != nil {
			return notFound()
		}
		card.Status = "revoked"
		card.UpdatedAt = s.now()
		if e = tx.Cards().Save(scoped, card); e != nil {
			return e
		}
		return s.append(scoped, tx, "card_revoked", cardID, map[string]any{"reason": reason, "license_unchanged": true})
	})
}

type ChannelInput struct {
	Name           string   `json:"name"`
	Enabled        bool     `json:"enabled"`
	AllowedPlanIDs []string `json:"allowed_plan_ids"`
}
type ChannelCredentials struct {
	Channel SalesChannel `json:"channel"`
	Secret  string       `json:"secret"`
}

func (s *Service) SaveSalesChannel(ctx context.Context, appID, channelID string, in ChannelInput, rotate bool) (ChannelCredentials, error) {
	scoped, _, repo, e := s.applicationContext(ctx, appID, true)
	if e != nil {
		return ChannelCredentials{}, e
	}
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 160 || len(in.AllowedPlanIDs) == 0 || len(in.AllowedPlanIDs) > 1000 {
		return ChannelCredentials{}, invalid("请填写销售通道名称并选择允许的套餐")
	}
	var out ChannelCredentials
	e = repo.Update(scoped, func(tx Transaction) error {
		for _, id := range in.AllowedPlanIDs {
			if _, e := tx.Plans().Get(scoped, id); e != nil {
				return notFound()
			}
		}
		if channelID == "" {
			out.Channel = SalesChannel{Record: s.record(appID, "channel"), Kind: "signed_webhook"}
			rotate = true
		} else {
			var e error
			out.Channel, e = tx.Channels().Get(scoped, channelID)
			if e != nil {
				return notFound()
			}
		}
		out.Channel.Name = strings.TrimSpace(in.Name)
		out.Channel.Enabled = in.Enabled
		out.Channel.AllowedPlanIDs = in.AllowedPlanIDs
		out.Channel.UpdatedAt = s.now()
		if rotate {
			raw, e := randomHex(32)
			if e != nil {
				return e
			}
			out.Secret = raw
			cipher, e := yncrypto.EncryptString(s.cryptoKey(appID, "webhook"), raw)
			if e != nil {
				return e
			}
			out.Channel.SecretCipher = cipher
			out.Channel.Version++
		}
		if channelID == "" {
			e = tx.Channels().Insert(scoped, out.Channel)
		} else {
			e = tx.Channels().Save(scoped, out.Channel)
		}
		if e != nil {
			return e
		}
		return s.append(scoped, tx, "sales_channel_saved", out.Channel.ID, map[string]any{"secret_rotated": rotate, "enabled": in.Enabled})
	})
	return out, e
}
func (s *Service) ListSalesChannels(ctx context.Context, appID string) ([]SalesChannel, error) {
	scoped, _, repo, e := s.applicationContext(ctx, appID, false)
	if e != nil {
		return nil, e
	}
	var out []SalesChannel
	e = repo.View(scoped, func(v View) error { var e error; out, e = v.Channels().Find(scoped, Filter{Limit: 1000}); return e })
	return out, e
}

type WebhookInput struct {
	OriginalEventID string `json:"original_event_id"`
	EventID         string `json:"event_id"`
	Event           string `json:"event"`
	SaleInput
}

// Webhook signatures use a separate rotating channel credential. AppSecret is
// hash-only and is never repurposed as a recoverable HMAC key.
func (s *Service) ReceiveSale(ctx context.Context, appID, channelID, timestamp, signature string, body []byte) (SaleResult, error) {
	scoped, _, repo, e := s.runtimeContext(ctx, appID, Actor{Kind: "sales_channel", ID: channelID})
	if e != nil {
		return SaleResult{}, e
	}
	var in WebhookInput
	if json.Unmarshal(body, &in) != nil || in.EventID == "" || len(in.EventID) > 190 || (in.Event != "payment.confirmed" && in.Event != "payment.refunded") {
		return SaleResult{}, invalid("销售事件无效")
	}
	var stamp int64
	if _, e = fmt.Sscanf(timestamp, "%d", &stamp); e != nil || fmt.Sprint(stamp) != timestamp || s.now().Sub(time.Unix(stamp, 0)) > 2*time.Minute || time.Unix(stamp, 0).Sub(s.now()) > 2*time.Minute {
		return SaleResult{}, forbidden("REQUEST_EXPIRED", "销售请求已过期")
	}
	canonical, _ := json.Marshal(in)
	sum := sha256.Sum256(canonical)
	bodyHash := hex.EncodeToString(sum[:])
	var out SaleResult
	e = repo.Update(scoped, func(tx Transaction) error {
		app, e := tx.Application(scoped)
		if e != nil {
			return e
		}
		ch, e := tx.Channels().Get(scoped, channelID)
		if e != nil || !ch.Enabled {
			return forbidden("CHANNEL_DISABLED", "销售通道无效")
		}
		secret, e := yncrypto.DecryptString(s.cryptoKey(appID, "webhook"), ch.SecretCipher)
		if e != nil {
			return e
		}
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(timestamp + "\n" + string(body)))
		want := hex.EncodeToString(mac.Sum(nil))
		if subtle.ConstantTimeCompare([]byte(want), []byte(signature)) != 1 {
			return forbidden("SIGNATURE_INVALID", "销售回调签名无效")
		}
		receipts, e := tx.Webhooks().Find(scoped, Filter{ChannelID: channelID, ExternalID: in.EventID, Limit: 1})
		if e != nil {
			return e
		}
		if len(receipts) > 0 {
			if receipts[0].BodyHash != bodyHash {
				return conflict("IDEMPOTENCY_CONFLICT", "事件 ID 已被不同内容使用")
			}
			order, e := tx.Orders().Get(scoped, receipts[0].OrderID)
			out = SaleResult{Order: order, Cards: []CardDelivery{}, Replayed: true}
			return e
		}
		if in.Event == "payment.refunded" {
			orders, e := tx.Orders().Find(scoped, Filter{ChannelID: channelID, ExternalID: in.OriginalEventID, Limit: 2})
			if e != nil {
				return e
			}
			if len(orders) != 1 {
				return invalid("退款 original_event_id 必须填写原 payment.confirmed 的 event_id")
			}
			order := orders[0]
			if e = s.refundOrder(scoped, tx, &order, "signed sales refund"); e != nil {
				return e
			}
			out.Order = order
		} else {
			if e = validateSale(in.SaleInput); e != nil {
				return e
			}
			if in.Terms != nil {
				return invalid("销售回调不得覆盖套餐规格")
			}
			if !contains(ch.AllowedPlanIDs, in.PlanID) {
				return forbidden("PLAN_NOT_GRANTED", "通道没有此套餐的销售权限")
			}
			plan, e := tx.Plans().Get(scoped, in.PlanID)
			if e != nil {
				return notFound()
			}
			if !plan.Enabled {
				return forbidden("PLAN_DISABLED", "套餐已停用")
			}
			order := SalesOrder{Record: s.record(appID, "order"), ChannelID: channelID, ExternalID: in.EventID, PlanID: plan.ID, CustomerRef: in.CustomerRef, OwnerUserID: in.OwnerUserID, Quantity: in.Quantity, AmountCents: in.AmountCents, Currency: plan.Currency, Status: "pending", Terms: plan.Terms, DistributeCards: in.DistributeCards, CardDigests: in.CardDigests}
			if e = tx.Orders().Insert(scoped, order); e != nil {
				return e
			}
			out, e = s.issueOrder(scoped, tx, app, order, in.Purpose, in.RedeemUntil)
			if e != nil {
				return e
			}
		}
		receipt := WebhookReceipt{Record: s.record(appID, "webhook"), ChannelID: channelID, ExternalID: in.EventID, BodyHash: bodyHash, OrderID: out.Order.ID, Result: in.Event}
		if e = tx.Webhooks().Insert(scoped, receipt); e != nil {
			return e
		}
		return s.append(scoped, tx, "sales_webhook", out.Order.ID, map[string]any{"event": in.Event, "event_id": in.EventID, "quantity": out.Order.Quantity})
	})
	if e != nil {
		return SaleResult{}, s.failure(scoped, repo, "sales_webhook", channelID, e)
	}
	return out, nil
}

func randomHex(size int) (string, error) {
	raw := make([]byte, size)
	if _, e := rand.Read(raw); e != nil {
		return "", e
	}
	return hex.EncodeToString(raw), nil
}

func (s *Service) cardHash(appID, raw string) string {
	sum := sha256.Sum256([]byte(normalizeCard(raw)))
	return s.hash("card", appID, hex.EncodeToString(sum[:]))
}

func (s *Service) refundOrder(ctx context.Context, tx Transaction, order *SalesOrder, reason string) error {
	var e error
	if order.Status == "refunded" {
		return nil
	}
	if order.Status != "confirmed" {
		return conflict("ORDER_NOT_CONFIRMED", "只有已确认的订单可确认退款")
	}

	order.Status = "refunded"
	order.UpdatedAt = s.now()
	if e = tx.Orders().Save(ctx, *order); e != nil {
		return e
	}
	for _, id := range order.LicenseIDs {
		l, e := tx.Licenses().Get(ctx, id)
		if e != nil {
			return e
		}
		l.Status = "revoked"
		l.Reason = "sales_refund"
		l.UpdatedAt = s.now()
		if e = tx.Licenses().Save(ctx, l); e != nil {
			return e
		}
	}
	affected := map[string]bool{}
	for _, sourceID := range order.LicenseIDs {
		cards, e := tx.Cards().Find(ctx, Filter{BoundLicenseID: sourceID, Limit: 1000})
		if e != nil {
			return e
		}
		for _, card := range cards {
			now := s.now()
			card.RefundedAt = &now
			card.Status = "revoked"
			card.UpdatedAt = now
			if e = tx.Cards().Save(ctx, card); e != nil {
				return e
			}
			if card.RedeemedLicenseID != "" {
				affected[card.RedeemedLicenseID] = true
			}
		}
	}
	for target := range affected {
		if e = s.recomputeRenewals(ctx, tx, target); e != nil {
			return e
		}
	}
	if order.AgentID != "" {
		balance, e := tx.CreditAgent(ctx, order.AgentID, order.CostCents)
		if e != nil {
			return e
		}
		event := MoneyEvent{Record: s.record(order.ApplicationID, "money"), AgentID: order.AgentID, OrderID: order.ID, RequestID: "refund:" + order.ID, AmountCents: order.CostCents, BalanceAfter: balance, Kind: "refund"}
		if e = tx.MoneyEvents().Insert(ctx, event); e != nil {
			return e
		}
	}
	return s.append(ctx, tx, "sale_refunded", order.ID, map[string]any{"reason": reason, "cost_returned_cents": order.CostCents})
}
func (s *Service) RefundSale(ctx context.Context, appID, orderID, reason string) (SalesOrder, error) {
	scoped, _, repo, e := s.applicationContext(ctx, appID, true)
	if e != nil {
		return SalesOrder{}, e
	}
	if strings.TrimSpace(reason) == "" || len(reason) > 1000 {
		return SalesOrder{}, invalid("请先处理外部款项并填写确认退款原因")
	}
	p, _ := PrincipalFrom(ctx)
	var order SalesOrder
	e = repo.Update(scoped, func(tx Transaction) error {
		var e error
		order, e = tx.Orders().Get(scoped, orderID)
		if e != nil {
			return notFound()
		}
		if order.AgentID != "" && !privileged(p, true) {
			return forbidden("ROLE_FORBIDDEN", "代理成本退款只能由平台管理员确认")
		}
		return s.refundOrder(scoped, tx, &order, reason)
	})
	return order, e
}
