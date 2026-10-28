package licensing_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	core "github.com/yunnuo88520/yunnuo-license/backend/internal/licensing"
)

func TestSalesCallbackIsSignedIdempotentAndRefundRevokes(t *testing.T) {
	s, _, root, now := coreFixture(t)
	app := fullApplication(t, s, root)
	plan, e := s.SavePlan(root, app.AppID, "", core.PlanInput{Name: "Web", Terms: core.Terms{Model: "perpetual", BindingTypes: []string{"domain"}, UpdateDurationDays: 30}, Currency: "CNY"})
	if e != nil {
		t.Fatal(e)
	}
	ch, e := s.SaveSalesChannel(root, app.AppID, "", core.ChannelInput{Name: "Store", Enabled: true, AllowedPlanIDs: []string{plan.ID}}, false)
	if e != nil {
		t.Fatal(e)
	}
	callback := func(in core.WebhookInput, secret string) (core.SaleResult, error) {
		raw, _ := json.Marshal(in)
		stamp := fmt.Sprint(now.Unix())
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(stamp + "\n" + string(raw)))
		return s.ReceiveSale(context.Background(), app.AppID, ch.Channel.ID, stamp, hex.EncodeToString(mac.Sum(nil)), raw)
	}
	in := core.WebhookInput{EventID: "sale-123", Event: "payment.confirmed", SaleInput: core.SaleInput{PlanID: plan.ID, Quantity: 2, DistributeCards: true, AmountCents: 2000}}
	_, e = callback(in, "wrong")
	assertCode(t, e, "SIGNATURE_INVALID")
	first, e := callback(in, ch.Secret)
	if e != nil || len(first.Cards) != 2 {
		t.Fatal(first, e)
	}
	again, e := callback(in, ch.Secret)
	if e != nil || !again.Replayed || len(again.Cards) != 0 || first.Order.ID != again.Order.ID {
		t.Fatal("duplicate delivery or card plaintext retained", again, e)
	}
	in.Quantity = 3
	_, e = callback(in, ch.Secret)
	assertCode(t, e, "IDEMPOTENCY_CONFLICT")
	refund := core.WebhookInput{EventID: "refund-123", OriginalEventID: "sale-123", Event: "payment.refunded"}
	if _, e = callback(refund, ch.Secret); e != nil {
		t.Fatal(e)
	}
	licenses, e := s.ListLicenses(root, app.AppID)
	if e != nil || len(licenses) != 2 {
		t.Fatal(licenses, e)
	}
	for _, l := range licenses {
		if l.Status != "revoked" {
			t.Fatal("refunded license remained usable")
		}
	}
	cards, e := s.ListCards(root, app.AppID)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(cards)
	for _, c := range first.Cards {
		if strings.Contains(string(raw), c.Code) {
			t.Fatal("plaintext card persisted")
		}
	}
	if _, e = s.AuditIntegrity(root, app.AppID); e != nil {
		t.Fatal(e)
	}
}
func TestPendingSaleHasNoEntitlementBeforeConfirmation(t *testing.T) {
	s, _, root, _ := coreFixture(t)
	app := fullApplication(t, s, root)
	plan, e := s.SavePlan(root, app.AppID, "", core.PlanInput{Name: "Subscription", Terms: core.Terms{Model: "subscription", BindingTypes: []string{"domain"}, DurationDays: 30, UpdateDurationDays: 15}, Currency: "CNY"})
	if e != nil {
		t.Fatal(e)
	}
	sale, e := s.CreateSale(root, app.AppID, core.SaleInput{PlanID: plan.ID, Quantity: 2, DistributeCards: true, Reason: "awaiting receipt"}, false)
	if e != nil {
		t.Fatal(e)
	}
	licenses, e := s.ListLicenses(root, app.AppID)
	if e != nil || len(licenses) != 0 {
		t.Fatal("unpaid order created authorization", licenses, e)
	}
	confirmed, e := s.ConfirmSale(root, app.AppID, sale.Order.ID, "bank transfer received")
	if e != nil || len(confirmed.Cards) != 2 {
		t.Fatal(confirmed, e)
	}
	if _, e = s.ConfirmSale(root, app.AppID, sale.Order.ID, "duplicate"); e == nil {
		t.Fatal("order issued twice")
	}
}

func TestExternalCommerceKeepsCardsAndRetriesWithoutLosingDelivery(t *testing.T) {
	s, _, root, now := coreFixture(t)
	app := fullApplication(t, s, root)
	plan, e := s.SavePlan(root, app.AppID, "", core.PlanInput{Name: "Store delivery", Terms: core.Terms{Model: "perpetual", BindingTypes: []string{"installation"}, UpdateDurationDays: 30}, Currency: "CNY"})
	if e != nil {
		t.Fatal(e)
	}
	channel, e := s.SaveSalesChannel(root, app.AppID, "", core.ChannelInput{Name: "External store", Enabled: true, AllowedPlanIDs: []string{plan.ID}}, false)
	if e != nil {
		t.Fatal(e)
	}
	code := "YN2-78AACE8599E609AE8D04E40C658EF28F61BCB915"
	digest := sha256.Sum256([]byte(code))
	in := core.WebhookInput{EventID: "external-001", Event: "payment.confirmed", SaleInput: core.SaleInput{PlanID: plan.ID, Quantity: 1, DistributeCards: true, CardDigests: []string{hex.EncodeToString(digest[:])}}}
	call := func(pretty bool) (core.SaleResult, error) {
		var raw []byte
		if pretty {
			raw, _ = json.MarshalIndent(in, "", "  ")
		} else {
			raw, _ = json.Marshal(in)
		}
		stamp := fmt.Sprint(now.Unix())
		mac := hmac.New(sha256.New, []byte(channel.Secret))
		mac.Write([]byte(stamp + "\n" + string(raw)))
		return s.ReceiveSale(context.Background(), app.AppID, channel.Channel.ID, stamp, hex.EncodeToString(mac.Sum(nil)), raw)
	}
	initial, e := call(false)
	if e != nil || len(initial.Cards) != 1 || initial.Cards[0].Code != "" {
		t.Fatal("platform must not receive or retain external plaintext", initial, e)
	}
	// Simulate losing the first success response; the store still has its own
	// card and can retry a differently formatted but identical event safely.
	retry, e := call(true)
	if e != nil || !retry.Replayed || retry.Order.ID != initial.Order.ID {
		t.Fatal("callback formatting caused duplicate issuance", retry, e)
	}
	client := newInstall(t)
	if _, e = activate(t, s, client, *now, core.ActivateInput{AppID: app.AppID, CardCode: code, Binding: core.BindingInput{Type: "installation", Value: "store-customer"}, Form: "gui"}); e != nil {
		t.Fatal("externally delivered standard card did not activate", e)
	}
	orders, e := s.ListSales(root, app.AppID)
	if e != nil || len(orders) != 1 {
		t.Fatal(orders, e)
	}
	encoded, _ := json.Marshal(orders)
	if strings.Contains(string(encoded), code) {
		t.Fatal("external plaintext stored in an order")
	}
	cards, e := s.ListCards(root, app.AppID)
	if e != nil || len(cards) != 1 || cards[0].Source != "sales_system" {
		t.Fatal(cards, e)
	}
}

func TestMaximumExternalCardBatchSurvivesStorageAndRetry(t *testing.T) {
	s, _, root, now := coreFixture(t)
	app := fullApplication(t, s, root)
	plan, e := s.SavePlan(root, app.AppID, "", core.PlanInput{Name: "Bulk commerce", Terms: core.Terms{Model: "perpetual", BindingTypes: []string{"installation"}}})
	if e != nil {
		t.Fatal(e)
	}
	channel, e := s.SaveSalesChannel(root, app.AppID, "", core.ChannelInput{Name: "Bulk channel", Enabled: true, AllowedPlanIDs: []string{plan.ID}}, false)
	if e != nil {
		t.Fatal(e)
	}
	digests := make([]string, 1000)
	for i := range digests {
		sum := sha256.Sum256([]byte(fmt.Sprintf("CONTROLLED-TEST-CARD-%06d", i)))
		digests[i] = hex.EncodeToString(sum[:])
	}
	in := core.WebhookInput{EventID: "bulk-1000-cards", Event: "payment.confirmed", SaleInput: core.SaleInput{PlanID: plan.ID, Quantity: 1000, DistributeCards: true, CardDigests: digests}}
	raw, e := json.Marshal(in)
	if e != nil {
		t.Fatal(e)
	}
	if len(raw) > 128<<10 {
		t.Fatal("documented batch exceeds callback transport limit")
	}
	stamp := fmt.Sprint(now.Unix())
	mac := hmac.New(sha256.New, []byte(channel.Secret))
	mac.Write([]byte(stamp + "\n" + string(raw)))
	signature := hex.EncodeToString(mac.Sum(nil))
	first, e := s.ReceiveSale(context.Background(), app.AppID, channel.Channel.ID, stamp, signature, raw)
	if e != nil {
		t.Fatal(e)
	}
	if len(first.Cards) != 1000 || len(first.Order.LicenseIDs) != 1000 {
		t.Fatal("batch truncated")
	}
	retry, e := s.ReceiveSale(context.Background(), app.AppID, channel.Channel.ID, stamp, signature, raw)
	if e != nil || !retry.Replayed || len(retry.Order.LicenseIDs) != 1000 {
		t.Fatal("large order could not be stored/replayed", e)
	}
	licenses, e := s.ListLicenses(root, app.AppID)
	if e != nil || len(licenses) != 1000 {
		t.Fatal("duplicate or missing licenses", len(licenses), e)
	}
}
