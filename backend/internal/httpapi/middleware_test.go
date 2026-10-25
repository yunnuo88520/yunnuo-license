package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
)

func TestFixedWindowLimiter(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	limiter := newFixedWindowLimiter(2, time.Minute)
	limiter.now = func() time.Time { return now }

	if allowed, _ := limiter.Allow("client-a"); !allowed {
		t.Fatal("first request should be allowed")
	}
	if allowed, _ := limiter.Allow("client-a"); !allowed {
		t.Fatal("second request should be allowed")
	}
	if allowed, retryAfter := limiter.Allow("client-a"); allowed || retryAfter != time.Minute {
		t.Fatalf("third request should be limited for one minute, allowed=%v retry=%s", allowed, retryAfter)
	}
	if allowed, _ := limiter.Allow("client-b"); !allowed {
		t.Fatal("different client should have an independent window")
	}

	now = now.Add(time.Minute)
	if allowed, _ := limiter.Allow("client-a"); !allowed {
		t.Fatal("request should be allowed after window reset")
	}
}

func TestRateLimitMiddlewareReturnsRetryAfter(t *testing.T) {
	limiter := newFixedWindowLimiter(1, time.Minute)
	handler := withRateLimit(limiter, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	first := httptest.NewRecorder()
	handler(first, httptest.NewRequest(http.MethodGet, "/limited", nil))
	if first.Code != http.StatusNoContent {
		t.Fatalf("first request returned %d", first.Code)
	}

	second := httptest.NewRecorder()
	handler(second, httptest.NewRequest(http.MethodGet, "/limited", nil))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("limited request returned %d", second.Code)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header")
	}
}

func TestFailureLockoutBlocksAfterThreshold(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	lock := newFailureLockout(3, 10*time.Minute)
	lock.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if allowed, _ := lock.Allow("1.2.3.4"); !allowed {
			t.Fatalf("attempt %d should stay allowed before threshold", i+1)
		}
		lock.Fail("1.2.3.4")
	}
	if allowed, retryAfter := lock.Allow("1.2.3.4"); allowed || retryAfter <= 0 {
		t.Fatalf("expected lockout after third failure, allowed=%v retry=%s", allowed, retryAfter)
	}
	if allowed, _ := lock.Allow("5.6.7.8"); !allowed {
		t.Fatal("other IPs must not be affected")
	}

	now = now.Add(10 * time.Minute)
	if allowed, _ := lock.Allow("1.2.3.4"); !allowed {
		t.Fatal("lockout should expire with the window")
	}
}

func TestRecordClientFailureIgnoresPolicyErrors(t *testing.T) {
	lock := newFailureLockout(2, time.Minute)

	recordClientFailure(lock, "1.2.3.4", service.ErrCardInvalid)
	recordClientFailure(lock, "1.2.3.4", service.ErrLicenseNotFound)
	if allowed, _ := lock.Allow("1.2.3.4"); allowed {
		t.Fatal("credential failures should count toward lockout")
	}

	other := newFailureLockout(2, time.Minute)
	for i := 0; i < 5; i++ {
		recordClientFailure(other, "1.2.3.4", service.ErrLicenseExpired)
		recordClientFailure(other, "1.2.3.4", service.ErrDeviceLimitExceeded)
	}
	if allowed, _ := other.Allow("1.2.3.4"); !allowed {
		t.Fatal("policy errors must never lock out legitimate clients")
	}
}

func TestClientFailureLockMiddlewareReturns429(t *testing.T) {
	lock := newFailureLockout(1, time.Minute)
	handler := withClientFailureLock(lock, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	first := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/licenses/activate", nil)
	req.RemoteAddr = "203.0.113.9:12345"
	handler(first, req)
	if first.Code != http.StatusNoContent {
		t.Fatalf("first request returned %d", first.Code)
	}
	lock.Fail("203.0.113.9")

	second := httptest.NewRecorder()
	handler(second, req)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("locked request returned %d", second.Code)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header")
	}
}

func TestPublicQueryProgressiveCaptchaGate(t *testing.T) {
	handler, _ := emailTestHandler(t)
	body := func(extra map[string]string) *strings.Reader {
		payload := map[string]string{"query": "203.0.113.42", "query_type": "ip"}
		for k, v := range extra {
			payload[k] = v
		}
		raw, _ := json.Marshal(payload)
		return strings.NewReader(string(raw))
	}
	post := func(payload *strings.Reader) int {
		req := httptest.NewRequest(http.MethodPost, "/v1/licenses/query", payload)
		req.RemoteAddr = "198.51.100.99:7777"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	for i := 0; i < 15; i++ {
		if code := post(body(nil)); code != http.StatusNotFound {
			t.Fatalf("query %d within free budget should reach the service (404 not-found), got %d", i+1, code)
		}
	}
	if code := post(body(nil)); code != http.StatusUnauthorized {
		t.Fatalf("query beyond free budget should require captcha (401), got %d", code)
	}
	if code := post(body(map[string]string{"captcha_id": "cap_bogus", "captcha_code": "AAAA"})); code != http.StatusUnauthorized {
		t.Fatalf("bogus captcha should be rejected (401), got %d", code)
	}
}
