package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
)

func TestApplicationCredentialsRequireOwnershipAndTokenScope(t *testing.T) {
	ctx := context.Background()
	svc := newHTTPTestService(t)
	if _, _, err := svc.EnsureBootstrapAdmin(ctx, "root", "root-password", "Root"); err != nil {
		t.Fatal(err)
	}
	h := New(svc, "", "", "").Handler()
	root := loginAdmin(t, h, "root", "root-password")
	developers := []domain.AdminUser{}
	for _, name := range []string{"owner", "other"} {
		var user domain.AdminUser
		decodeData(t, performJSON(t, h, "POST", "/admin/users", root, map[string]string{"username": name, "password": "private-password-123", "role": "developer"}), &user)
		developers = append(developers, user)
	}
	owner := loginAdmin(t, h, "owner", "private-password-123")
	other := loginAdmin(t, h, "other", "private-password-123")
	var product domain.Product
	create := performJSONRequest(t, h, "POST", "/developer/products", owner, map[string]string{"name": "Ebox", "bind_mode": "domain"})
	decodeData(t, create.Body.Bytes(), &product)
	if create.Header().Get("Cache-Control") != "no-store" || len(product.AppSecret) != 32 {
		t.Fatal("creation secret was not returned privately")
	}
	path := "/developer/products/" + product.ID + "/credentials"
	performJSONStatus(t, h, "GET", path, other, nil, 404)
	performJSONStatus(t, h, "POST", path+"/reset", other, map[string]any{"expected_version": 1, "reason": "forged reset"}, 404)
	performJSONStatus(t, h, "GET", path, "", nil, 401)
	var status service.ApplicationCredentialStatus
	decodeData(t, performJSON(t, h, "GET", path, owner, nil), &status)
	if status.Version != 1 || !status.Configured {
		t.Fatal("owner cannot inspect credential status")
	}
	for _, endpoint := range []string{"/developer/products", "/admin/products"} {
		token := owner
		if endpoint == "/admin/products" {
			token = root
		}
		if raw := performJSON(t, h, "GET", endpoint, token, nil); strings.Contains(string(raw), product.AppSecret) || strings.Contains(string(raw), "app_secret") {
			t.Fatal("read endpoint leaked a secret")
		}
	}
	var appToken service.ApplicationTokenResponse
	decodeData(t, performJSON(t, h, "POST", "/v1/app/token", "", map[string]string{"grant_type": "client_credential", "appid": product.ID, "secret": product.AppSecret}), &appToken)
	var profile map[string]any
	decodeData(t, performJSON(t, h, "GET", "/v1/app/profile", appToken.AccessToken, nil), &profile)
	if profile["appid"] != product.ID {
		t.Fatal("wrong application token scope")
	}
	performJSONStatus(t, h, "GET", "/admin/products", appToken.AccessToken, nil, 401)
	performJSONStatus(t, h, "GET", "/developer/products", appToken.AccessToken, nil, 401)
	performJSONStatus(t, h, "GET", "/v1/app/profile", root, nil, 401)
	batch, err := svc.CreateCardBatch(ctx, service.CreateCardBatchInput{ProductID: product.ID, Name: "test", Quantity: 1, DurationDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	license, err := svc.Activate(ctx, service.ActivateInput{AppKey: product.ID, CardCode: batch.Codes[0], BindValue: "ebox.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	var checked service.LicenseResponse
	decodeData(t, performJSON(t, h, "POST", "/v1/app/licenses/verify", appToken.AccessToken, map[string]string{"license_no": license.LicenseNo, "bind_mode": "domain", "bind_value": "ebox.example.com"}), &checked)
	if !checked.CanUse {
		t.Fatal("server token could not check valid license")
	}
	performJSONStatus(t, h, "POST", "/v1/app/licenses/verify", appToken.AccessToken, map[string]string{"app_key": "yn0000000000000000", "license_no": license.LicenseNo, "bind_value": "ebox.example.com"}, 403)
	var reset service.ApplicationSecretResponse
	decodeData(t, performJSON(t, h, "POST", path+"/reset", owner, map[string]any{"expected_version": 1, "reason": "lost secret"}), &reset)
	performJSONStatus(t, h, "GET", "/v1/app/profile", appToken.AccessToken, nil, 401)
	performJSONStatus(t, h, "POST", path+"/reset", owner, map[string]any{"expected_version": 1, "reason": "stale reset"}, 409)
	var logs service.OperationPage
	decodeData(t, performJSON(t, h, "GET", "/admin/operation-logs?category=applications&page_size=5000", root, nil), &logs)
	raw, _ := json.Marshal(logs)
	for _, secret := range []string{product.AppSecret, reset.AppSecret, appToken.AccessToken} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("credential leaked in operation journal")
		}
	}
	found := false
	for _, log := range logs.Items {
		if strings.HasSuffix(log.Route, "/credentials/reset") && log.ActorType == "developer" && log.ActorID == developers[0].ID {
			found = true
		}
	}
	if !found {
		t.Fatal("secret reset is not attributable to its developer")
	}
}
