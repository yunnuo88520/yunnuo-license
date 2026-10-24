package httpapi

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
)

func releaseUploadRequest(t *testing.T, handler http.Handler, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	form.WriteField("version", "1.2.0")
	form.WriteField("notes", "Ebox update")
	file, err := form.CreateFormFile("package", "ebox.zip")
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(file, "Ebox package")
	form.Close()
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestDeveloperProductIsolationAndReleaseWorkflow(t *testing.T) {
	ctx := context.Background()
	svc := newHTTPTestService(t).WithReleaseDir(t.TempDir())
	if _, _, err := svc.EnsureBootstrapAdmin(ctx, "root", "secret123", "Root"); err != nil {
		t.Fatal(err)
	}
	var developers []domain.AdminUser
	for _, name := range []string{"xiaowang", "xiaoli"} {
		user, err := svc.CreateAdminUser(ctx, service.CreateAdminUserInput{Username: name, Password: "secret123", Role: domain.AdminRoleDeveloper})
		if err != nil {
			t.Fatal(err)
		}
		developers = append(developers, user)
	}
	handler := New(svc, "", "", "").Handler()
	root := loginAdmin(t, handler, "root", "secret123")
	login := func(username string) string {
		var result service.AdminLoginResponse
		decodeData(t, performJSON(t, handler, http.MethodPost, "/developer/login", "", map[string]string{"username": username, "password": "secret123"}), &result)
		return result.AccessToken
	}
	wang, li := login("xiaowang"), login("xiaoli")
	var p domain.Product
	decodeData(t, performJSON(t, handler, http.MethodPost, "/developer/products", wang, map[string]any{"name": "Ebox", "code": "EBOX", "bind_mode": "domain"}), &p)
	if p.DeveloperID != developers[0].ID {
		t.Fatal("incorrect owner")
	}
	var own, others []domain.Product
	decodeData(t, performJSON(t, handler, http.MethodGet, "/developer/products", wang, nil), &own)
	decodeData(t, performJSON(t, handler, http.MethodGet, "/developer/products", li, nil), &others)
	if len(own) != 1 || len(others) != 0 {
		t.Fatal("product listing leaked across developers")
	}
	base := "/developer/products/" + p.ID
	for _, path := range []string{"/updates", "/releases", "/licenses"} {
		performJSONStatus(t, handler, http.MethodGet, base+path, li, nil, http.StatusNotFound)
	}
	for _, path := range []string{"/updates", "/card-batches", "/releases/missing/publish"} {
		performJSONStatus(t, handler, http.MethodPost, base+path, li, map[string]any{"enabled": true}, http.StatusNotFound)
	}
	if response := releaseUploadRequest(t, handler, base+"/releases", li); response.Code != http.StatusNotFound {
		t.Fatalf("unauthorized upload: %d", response.Code)
	}
	for _, path := range []string{"/admin/products", "/admin/users", "/admin/licenses", "/admin/audit-logs"} {
		performJSONStatus(t, handler, http.MethodGet, path, wang, nil, http.StatusForbidden)
	}
	performJSONStatus(t, handler, http.MethodPost, "/admin/products/"+p.ID+"/developer", wang, map[string]string{"developer_id": developers[1].ID}, http.StatusForbidden)
	performJSONStatus(t, handler, http.MethodPost, "/developer/products", wang, map[string]string{"name": "Forged", "code": "FORGED", "developer_id": developers[1].ID}, http.StatusBadRequest)
	performJSON(t, handler, http.MethodPost, base+"/updates", wang, map[string]bool{"enabled": true})
	response := releaseUploadRequest(t, handler, base+"/releases", wang)
	if response.Code != http.StatusOK {
		t.Fatalf("upload: %s", response.Body.String())
	}
	var release domain.ProductRelease
	decodeData(t, response.Body.Bytes(), &release)
	performJSONStatus(t, handler, http.MethodPost, base+"/releases/"+release.ID+"/publish", li, nil, http.StatusNotFound)
	performJSON(t, handler, http.MethodPost, base+"/releases/"+release.ID+"/publish", wang, nil)
	var batch service.CreateCardBatchResult
	decodeData(t, performJSON(t, handler, http.MethodPost, base+"/card-batches", wang, map[string]any{"product_id": "ignored-body-product", "name": "Ebox trial", "quantity": 1, "duration_days": 30}), &batch)
	if batch.Batch.ProductID != p.ID {
		t.Fatal("body overrode scoped product")
	}
	var activated service.LicenseResponse
	decodeData(t, performJSON(t, handler, http.MethodPost, "/v1/licenses/activate", "", map[string]string{"app_key": p.AppKey, "card_code": batch.Codes[0], "bind_mode": "domain", "bind_value": "ebox.example.com"}), &activated)
	binding := map[string]string{"app_key": p.AppKey, "license_no": activated.LicenseNo, "bind_mode": "domain", "bind_value": "ebox.example.com"}
	check := map[string]string{"app_key": p.AppKey, "license_no": activated.LicenseNo, "bind_mode": "domain", "bind_value": "ebox.example.com", "current_version": "1.0.0"}
	var result service.CheckUpdateResult
	decodeData(t, performJSON(t, handler, http.MethodPost, "/v1/updates/check", "", check), &result)
	if !result.UpdateAvailable || result.Release.ID != release.ID {
		t.Fatal("update not offered")
	}
	download := performJSONRequest(t, handler, http.MethodPost, "/v1/updates/"+release.ID+"/download", "", binding)
	if download.Code != http.StatusOK || download.Body.String() != "Ebox package" || download.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("download: %d %s", download.Code, download.Body.String())
	}
	performJSONStatus(t, handler, http.MethodGet, "/v1/updates/"+release.ID+"/download", "", nil, http.StatusMethodNotAllowed)
	performJSON(t, handler, http.MethodPost, base+"/releases/"+release.ID+"/withdraw", wang, nil)
	performJSONStatus(t, handler, http.MethodPost, "/v1/updates/"+release.ID+"/download", "", binding, http.StatusNotFound)
	performJSON(t, handler, http.MethodPost, "/admin/products/"+p.ID+"/developer", root, map[string]string{"developer_id": developers[1].ID})
	performJSONStatus(t, handler, http.MethodGet, base+"/releases", wang, nil, http.StatusNotFound)
	performJSON(t, handler, http.MethodGet, base+"/releases", li, nil)
	performJSON(t, handler, http.MethodPost, "/admin/developers/"+developers[1].ID+"/disable", root, nil)
	performJSONStatus(t, handler, http.MethodGet, base+"/releases", li, nil, http.StatusForbidden)
}
