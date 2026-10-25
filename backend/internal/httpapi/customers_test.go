package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

func TestCustomerHTTPIsolationAndCategorizedOperationLogs(t *testing.T) {
	ctx := context.Background()
	svc := newHTTPTestService(t)
	if _, _, err := svc.EnsureBootstrapAdmin(ctx, "root", "root-password", "Root"); err != nil {
		t.Fatal(err)
	}
	h := New(svc, "", "", "").Handler()
	root := loginAdmin(t, h, "root", "root-password")
	users := map[string]domain.Customer{}
	tokens := map[string]string{}
	for _, name := range []string{"alice", "bob"} {
		var user domain.Customer
		decodeData(t, performJSON(t, h, "POST", "/admin/customers", root, map[string]string{"username": name, "password": "private-password-123"}), &user)
		users[name] = user
		var login service.CustomerLoginResponse
		decodeData(t, performJSON(t, h, "POST", "/user/login", "", map[string]string{"username": name, "password": "private-password-123"}), &login)
		tokens[name] = login.AccessToken
	}
	p, err := svc.CreateProduct(ctx, service.CreateProductInput{Name: "Ebox", Code: "OWN", BindMode: domain.BindDomain})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := svc.CreateCardBatch(ctx, service.CreateCardBatchInput{ProductID: p.ID, Name: "Users", Quantity: 2, DurationDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	var lic service.LicenseResponse
	performJSONStatus(t, h, "POST", "/user/activate", tokens["alice"], map[string]string{"application_id": "prod_wrong", "card_code": batch.Codes[0], "bind_value": "alice.example.com"}, 404)
	decodeData(t, performJSON(t, h, "POST", "/user/activate", tokens["alice"], map[string]string{"card_code": batch.Codes[0], "bind_value": "alice.example.com"}), &lic)
	performJSONStatus(t, h, "GET", "/user/licenses/"+lic.LicenseNo, tokens["bob"], nil, 404)
	performJSONStatus(t, h, "POST", "/user/licenses/"+lic.LicenseNo+"/renew", tokens["bob"], map[string]string{"card_code": batch.Codes[1], "binding_id": "fake"}, 404)
	performJSONStatus(t, h, "POST", "/admin/licenses/"+lic.LicenseNo+"/owner", tokens["alice"], map[string]string{"user_id": users["bob"].ID, "reason": "self transfer"}, 401)
	performJSONStatus(t, h, "GET", "/admin/customers", tokens["alice"], nil, 401)
	var bob []domain.CustomerLicense
	decodeData(t, performJSON(t, h, "GET", "/user/licenses?user_id="+users["alice"].ID, tokens["bob"], nil), &bob)
	if len(bob) != 0 {
		t.Fatal("query parameters changed authenticated identity")
	}
	performJSONStatus(t, h, "POST", "/user/activate", tokens["bob"], map[string]string{"application_code": p.Code, "card_code": batch.Codes[0], "bind_value": "bob.example.com"}, 403)
	var view domain.CustomerLicense
	decodeData(t, performJSON(t, h, "GET", "/user/licenses/"+lic.LicenseNo, tokens["alice"], nil), &view)
	performJSONStatus(t, h, "POST", "/user/licenses/"+lic.LicenseNo+"/rebind", tokens["alice"], map[string]string{"binding_id": view.Bindings[0].ID, "new_value": "next.example.com", "request_id": "disabled-test", "reason": "migration"}, 403)
	var logs service.OperationPage
	decodeData(t, performJSON(t, h, "GET", "/admin/operation-logs?category=binding&result=failed", root, nil), &logs)
	if len(logs.Items) != 1 || logs.Items[0].ActorType != "customer" || logs.Items[0].ActorID != users["alice"].ID || logs.Items[0].ErrorCode != "REBIND_DISABLED" || logs.Items[0].ObjectID != lic.LicenseNo {
		t.Fatalf("missing classified denial: %+v", logs)
	}
	decodeData(t, performJSON(t, h, "GET", "/admin/operation-logs?page_size=5000", root, nil), &logs)
	raw, _ := json.Marshal(logs)
	for _, secret := range []string{"private-password-123", batch.Codes[0], tokens["alice"], root} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("operation log exposed credentials")
		}
	}
	denied := false
	for _, l := range logs.Items {
		if l.ActorID == users["bob"].ID && l.HTTPStatus == 404 {
			denied = true
		}
	}
	if !denied {
		t.Fatal("cross-user access denial was not attributed")
	}
	export := performJSONRequest(t, h, "GET", "/admin/operation-logs/export?category=binding", root, nil)
	if export.Code != 200 || !strings.Contains(export.Header().Get("Content-Type"), "text/csv") || !strings.Contains(export.Body.String(), "REBIND_DISABLED") {
		t.Fatal("filtered operation export failed")
	}
}

func TestOperationAuditUnavailablePreventsMutation(t *testing.T) {
	ctx := context.Background()
	dsn := "file:" + filepath.Join(t.TempDir(), "failure.db") + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	st, err := store.Open(ctx, "sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.Migrate(ctx, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	svc := service.New(st, []byte("pepper"), []byte("0123456789abcdef0123456789abcdef"))
	defer svc.Close()
	if _, _, err = svc.EnsureBootstrapAdmin(ctx, "root", "root-password", "Root"); err != nil {
		t.Fatal(err)
	}
	h := New(svc, "", "", "").Handler()
	root := loginAdmin(t, h, "root", "root-password")
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err = raw.Exec(`DROP TABLE operation_logs`); err != nil {
		t.Fatal(err)
	}
	response := performJSONRequest(t, h, http.MethodPost, "/admin/customers", root, map[string]string{"username": "blocked", "password": "valid-password-123"})
	if response.Code != 503 || !strings.Contains(response.Body.String(), "AUDIT_UNAVAILABLE") {
		t.Fatalf("logging failure was not fail-closed: %d", response.Code)
	}
	users, err := svc.Customers(ctx)
	if err != nil || len(users) != 0 {
		t.Fatal("business mutation ran without its operation journal")
	}
}
