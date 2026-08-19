package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
)

func TestClientIPOnlyTrustsForwardingHeadersWhenConfigured(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "192.0.2.10:12345"
	request.Header.Set("X-Forwarded-For", "203.0.113.20, 198.51.100.4")

	api := New(nil, "", "", "")
	if got := api.clientIP(request); got != "192.0.2.10" {
		t.Fatalf("untrusted forwarding header changed client IP: %q", got)
	}
	api.WithTrustedProxyHeaders(true)
	if got := api.clientIP(request); got != "203.0.113.20" {
		t.Fatalf("trusted forwarding header was not used: %q", got)
	}
}

func TestAdminRiskControlEndpoints(t *testing.T) {
	ctx := context.Background()
	svc := newHTTPTestService(t)
	if _, _, err := svc.EnsureBootstrapAdmin(ctx, "root", "secret123", "Root Admin"); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}
	product, err := svc.CreateProduct(ctx, service.CreateProductInput{Name: "Risk Product", Code: "RSK"})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	handler := New(svc, "", "", "").Handler()
	token := loginAdmin(t, handler, "root", "secret123")

	createdBody := performJSON(t, handler, http.MethodPost, "/admin/risk/blocks", token, map[string]any{
		"product_id": product.ID,
		"kind":       domain.RiskBlockDevice,
		"value":      "DEVICE-SECRET-123456",
		"reason":     "compromised device",
	})
	var block domain.RiskBlock
	decodeData(t, createdBody, &block)
	if block.ID == "" || block.ValueMasked == "DEVICE-SECRET-123456" {
		t.Fatalf("unexpected risk block response: %#v", block)
	}

	listBody := performJSON(t, handler, http.MethodGet, "/admin/risk/blocks?status=active", token, nil)
	var blocks []domain.RiskBlock
	decodeData(t, listBody, &blocks)
	if len(blocks) != 1 || blocks[0].ID != block.ID {
		t.Fatalf("unexpected risk block list: %#v", blocks)
	}
	performJSON(t, handler, http.MethodGet, "/admin/risk/summary", token, nil)
	performJSON(t, handler, http.MethodPost, "/admin/risk/blocks/"+block.ID+"/disable", token, nil)

	listBody = performJSON(t, handler, http.MethodGet, "/admin/risk/blocks?status=disabled", token, nil)
	decodeData(t, listBody, &blocks)
	if len(blocks) != 1 || blocks[0].Status != domain.RiskStatusDisabled {
		t.Fatalf("risk block was not disabled: %#v", blocks)
	}
}
