package ynlicense

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientActivateAndAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/licenses/activate" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("User-Agent"); got != "example-product/2.0" {
			t.Fatalf("unexpected user agent: %q", got)
		}
		var input map[string]any
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if input["app_key"] != "app_test" || input["bind_value"] != "machine-A" {
			t.Fatalf("unexpected request payload: %#v", input)
		}
		w.Header().Set("Content-Type", "application/json")
		if input["card_code"] == "bad" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"success":false,"request_id":"req_error","error":{"code":"CARD_INVALID","message":"card invalid"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"request_id":"req_ok","data":{"license_no":"lic_test","status":"active","server_time":"2026-08-10T03:00:00Z"}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/", "app_test", WithUserAgent("example-product/2.0"))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	result, err := client.Activate(context.Background(), ActivateRequest{
		CardCode:  "YN-TEST",
		BindMode:  "device",
		BindValue: "machine-A",
	})
	if err != nil || result.LicenseNo != "lic_test" || result.ServerTime.IsZero() {
		t.Fatalf("activate: result=%#v err=%v", result, err)
	}

	_, err = client.Activate(context.Background(), ActivateRequest{
		CardCode:  "bad",
		BindMode:  "device",
		BindValue: "machine-A",
	})
	if !IsAPIErrorCode(err, "CARD_INVALID") {
		t.Fatalf("expected CARD_INVALID, got %T %v", err, err)
	}
	apiErr := err.(*APIError)
	if apiErr.HTTPStatus != http.StatusNotFound || apiErr.RequestID != "req_error" {
		t.Fatalf("unexpected API error: %#v", apiErr)
	}
}

func TestClientTimeoutAndConfiguration(t *testing.T) {
	if _, err := NewClient("not-a-url", "app_test"); err == nil {
		t.Fatal("expected invalid base URL error")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte(`{"success":true,"data":{}}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "app_test", WithHTTPClient(&http.Client{Timeout: 10 * time.Millisecond}))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	_, err = client.Heartbeat(context.Background(), HeartbeatRequest{LicenseNo: "lic_test", BindMode: "device", BindValue: "machine-A"})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestClientLifecycleEndpoints(t *testing.T) {
	seen := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path]++
		var input map[string]any
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatalf("decode %s request: %v", r.URL.Path, err)
		}
		if input["app_key"] != "app_test" {
			t.Fatalf("%s missing app key: %#v", r.URL.Path, input)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/licenses/verify", "/v1/licenses/renew":
			_, _ = w.Write([]byte(`{"success":true,"data":{"license_no":"lic_test","status":"active","server_time":"2026-08-10T03:00:00Z"}}`))
		case "/v1/licenses/heartbeat":
			_, _ = w.Write([]byte(`{"success":true,"data":{"accepted":true,"server_time":"2026-08-10T03:00:00Z"}}`))
		case "/v1/licenses/unbind":
			_, _ = w.Write([]byte(`{"success":true,"data":{"unbound":true,"license_no":"lic_test","server_time":"2026-08-10T03:00:00Z"}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "app_test")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	ctx := context.Background()
	if _, err := client.Verify(ctx, VerifyRequest{LicenseNo: "lic_test", BindMode: "device", BindValue: "machine-A"}); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if result, err := client.Heartbeat(ctx, HeartbeatRequest{LicenseNo: "lic_test", BindMode: "device", BindValue: "machine-A"}); err != nil || !result.Accepted {
		t.Fatalf("heartbeat: result=%#v err=%v", result, err)
	}
	if _, err := client.Renew(ctx, RenewRequest{LicenseNo: "lic_test", RenewCardCode: "YN-RENEW", BindMode: "device", BindValue: "machine-A"}); err != nil {
		t.Fatalf("renew: %v", err)
	}
	if result, err := client.Unbind(ctx, UnbindRequest{LicenseNo: "lic_test", BindMode: "device", BindValue: "machine-A"}); err != nil || !result.Unbound {
		t.Fatalf("unbind: result=%#v err=%v", result, err)
	}
	for _, path := range []string{"/v1/licenses/verify", "/v1/licenses/heartbeat", "/v1/licenses/renew", "/v1/licenses/unbind"} {
		if seen[path] != 1 {
			t.Fatalf("expected one request to %s, got %d", path, seen[path])
		}
	}
}
