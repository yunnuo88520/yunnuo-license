package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
