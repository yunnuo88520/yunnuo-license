package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
)

func TestDeveloperCanConfigureSeparateRightsBeforeIssuingCards(t *testing.T) {
	ctx := context.Background()
	svc := newHTTPTestService(t)
	if _, err := svc.CreateAdminUser(ctx, service.CreateAdminUserInput{Username: "developer", Password: "secret123", Role: domain.AdminRoleDeveloper}); err != nil {
		t.Fatal(err)
	}
	handler := New(svc, "", "", "").Handler()
	token := loginAdmin(t, handler, "developer", "secret123")
	var p domain.Product
	decodeData(t, performJSON(t, handler, http.MethodPost, "/developer/products", token, map[string]any{"name": "Ebox", "code": "EBOX", "license_mode": "subscription"}), &p)
	base := "/developer/products/" + p.ID
	var policy service.LicensePolicy
	decodeData(t, performJSON(t, handler, http.MethodGet, base+"/license-policy", token, nil), &policy)
	if policy.Locked || policy.LicenseMode != domain.LicenseSubscription {
		t.Fatalf("default policy: %+v", policy)
	}
	performJSON(t, handler, http.MethodPost, base+"/license-policy", token, map[string]any{"license_mode": "perpetual", "update_duration_days": 7, "expire_grace_days": 0})
	var batch service.CreateCardBatchResult
	decodeData(t, performJSON(t, handler, http.MethodPost, base+"/card-batches", token, map[string]any{"name": "Ebox", "quantity": 1, "duration_days": 30}), &batch)
	var license service.LicenseResponse
	decodeData(t, performJSON(t, handler, http.MethodPost, "/v1/licenses/activate", "", map[string]string{"app_key": p.AppKey, "card_code": batch.Codes[0], "bind_mode": "device", "bind_value": "fixture-device"}), &license)
	if license.LicenseMode != domain.LicensePerpetual || !license.CanUse || !license.CanUpdate || license.UsageExpiredAt != nil || license.UpdatesExpiredAt == nil {
		t.Fatalf("wrong wire rights: %+v", license)
	}
	decodeData(t, performJSON(t, handler, http.MethodGet, base+"/license-policy", token, nil), &policy)
	if !policy.Locked {
		t.Fatal("issued terms not locked")
	}
	performJSONStatus(t, handler, http.MethodPost, base+"/license-policy", token, map[string]any{"license_mode": "subscription"}, http.StatusConflict)
}
