package httpapi

import (
	"context"
	"errors"
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
)

type contextKey string

const requestIDKey contextKey = "request_id"

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := yncrypto.NewID("req")
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requestIDFrom(r *http.Request) string {
	id, _ := r.Context().Value(requestIDKey).(string)
	return id
}

type rateWindow struct {
	startedAt time.Time
	count     int
}

type fixedWindowLimiter struct {
	mu      sync.Mutex
	entries map[string]rateWindow
	limit   int
	window  time.Duration
	now     func() time.Time
}

func newFixedWindowLimiter(limit int, window time.Duration) *fixedWindowLimiter {
	return &fixedWindowLimiter{
		entries: make(map[string]rateWindow),
		limit:   limit,
		window:  window,
		now:     time.Now,
	}
}

func (l *fixedWindowLimiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	entry, ok := l.entries[key]
	if !ok || now.Sub(entry.startedAt) >= l.window {
		l.entries[key] = rateWindow{startedAt: now, count: 1}
		l.cleanup(now)
		return true, 0
	}
	if entry.count >= l.limit {
		return false, l.window - now.Sub(entry.startedAt)
	}
	entry.count++
	l.entries[key] = entry
	return true, 0
}

func (l *fixedWindowLimiter) cleanup(now time.Time) {
	if len(l.entries) < 4096 {
		return
	}
	for key, entry := range l.entries {
		if now.Sub(entry.startedAt) >= l.window {
			delete(l.entries, key)
		}
	}
}

// failureLockout blocks a key after threshold recorded failures inside window.
// Only credential-shaped failures should be recorded (see recordClientFailure),
// so policy errors like LICENSE_EXPIRED never lock out legitimate clients.
type failureLockout struct {
	mu        sync.Mutex
	entries   map[string]rateWindow
	threshold int
	window    time.Duration
	now       func() time.Time
}

func newFailureLockout(threshold int, window time.Duration) *failureLockout {
	return &failureLockout{
		entries:   make(map[string]rateWindow),
		threshold: threshold,
		window:    window,
		now:       time.Now,
	}
}

func (l *failureLockout) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	entry, ok := l.entries[key]
	if !ok || now.Sub(entry.startedAt) >= l.window {
		return true, 0
	}
	if entry.count >= l.threshold {
		return false, l.window - now.Sub(entry.startedAt)
	}
	return true, 0
}

func (l *failureLockout) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	entry, ok := l.entries[key]
	if !ok || now.Sub(entry.startedAt) >= l.window {
		l.entries[key] = rateWindow{startedAt: now, count: 1}
		l.cleanup(now)
		return
	}
	entry.count++
	l.entries[key] = entry
}

func (l *failureLockout) cleanup(now time.Time) {
	if len(l.entries) < 4096 {
		return
	}
	for key, entry := range l.entries {
		if now.Sub(entry.startedAt) >= l.window {
			delete(l.entries, key)
		}
	}
}

// recordClientFailure reports a service error to the lockout when it looks
// like credential probing (wrong card code or unknown license), so that
// repeated guesses lock the source IP without penalizing policy failures.
func recordClientFailure(lock *failureLockout, clientIP string, err error) {
	if err == nil {
		return
	}
	var appErr service.AppError
	if !errors.As(err, &appErr) {
		return
	}
	switch appErr.Code {
	case "INVALID_CREDENTIALS", "MFA_REQUIRED", "MFA_CODE_INVALID", "MFA_CHALLENGE_INVALID", "CARD_INVALID", "CARD_NOT_FOUND", "LICENSE_NOT_FOUND", "INVALID_APP_KEY":
		lock.Fail(clientIP)
	}
}

// withClientFailureLock rejects the request while the client IP is locked out.
func withClientFailureLock(lock *failureLockout, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if allowed, retryAfter := lock.Allow(remoteClientIP(r)); !allowed {
			seconds := int(math.Ceil(retryAfter.Seconds()))
			w.Header().Set("Retry-After", strconv.Itoa(max(seconds, 1)))
			writeError(w, r, service.ErrRateLimited)
			return
		}
		next(w, r)
	}
}

func withRateLimit(limiter *fixedWindowLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		allowed, retryAfter := limiter.Allow(remoteClientIP(r))
		if !allowed {
			seconds := int(math.Ceil(retryAfter.Seconds()))
			w.Header().Set("Retry-After", strconv.Itoa(max(seconds, 1)))
			writeError(w, r, service.ErrRateLimited)
			return
		}
		next(w, r)
	}
}

func remoteClientIP(r *http.Request) string {
	if ip, ok := r.Context().Value(clientIPContext).(string); ok {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	if r.RemoteAddr != "" {
		return r.RemoteAddr
	}
	return "unknown"
}
