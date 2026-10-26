package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

var operationNames = []string{"activation", "verification", "heartbeat", "renewal", "update_check", "update_download", "management", "user", "developer", "public", "other"}

var latencyBounds = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 120}

type requestMetric struct {
	requests, errors, denials, nanos atomic.Uint64
	buckets                          [13]atomic.Uint64
}
type metricsWriter struct {
	http.ResponseWriter
	status int
}

func (w *metricsWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
		w.ResponseWriter.WriteHeader(status)
	}
}
func (w *metricsWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(b)
}
func (w *metricsWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func metricGroup(path string) int {
	for i, p := range []string{"/v1/licenses/activate", "/v1/licenses/verify", "/v1/licenses/heartbeat", "/v1/licenses/renew", "/v1/updates/check"} {
		if path == p {
			return i
		}
	}
	if strings.HasPrefix(path, "/v1/updates/") {
		return 5
	}
	if strings.HasPrefix(path, "/admin/") {
		return 6
	}
	if strings.HasPrefix(path, "/user/") {
		return 7
	}
	if strings.HasPrefix(path, "/developer/") {
		return 8
	}
	if strings.HasPrefix(path, "/v1/") {
		return 9
	}
	return 10
}
func (api *API) observeRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		capture := &metricsWriter{ResponseWriter: w}
		next.ServeHTTP(capture, r)
		metric := &api.metrics[metricGroup(r.URL.Path)]
		metric.requests.Add(1)
		elapsed := time.Since(start)
		metric.nanos.Add(uint64(elapsed))
		for i, bound := range latencyBounds {
			if elapsed.Seconds() <= bound {
				metric.buckets[i].Add(1)
			}
		}
		if capture.status >= 500 {
			metric.errors.Add(1)
		} else if capture.status >= 400 {
			metric.denials.Add(1)
		}
	})
}
func (api *API) ready(w http.ResponseWriter, r *http.Request) {
	status := api.service.Readiness(r.Context())
	if !status.Ready {
		w.Header().Set("Retry-After", "5")
		writeJSON(w, 503, map[string]any{"success": false, "data": status})
		return
	}
	writeOK(w, r, status)
}
func (api *API) WithMetricsToken(token string) *API { api.metricsToken = token; return api }
func (api *API) exposeMetrics(w http.ResponseWriter, r *http.Request) {
	if api.metricsToken == "" {
		http.NotFound(w, r)
		return
	}
	expected := sha256.Sum256([]byte(api.metricsToken))
	provided := sha256.Sum256([]byte(bearerToken(r)))
	if subtle.ConstantTimeCompare(expected[:], provided[:]) != 1 {
		writeError(w, r, service.ErrPermissionDenied)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	status := api.service.Readiness(r.Context())
	ready := 0
	if status.Ready {
		ready = 1
	}
	fmt.Fprintf(w, "# TYPE yunnuo_ready gauge\nyunnuo_ready %d\n", ready)
	fmt.Fprintln(w, "# TYPE yunnuo_requests_total counter\n# TYPE yunnuo_errors_total counter\n# TYPE yunnuo_denials_total counter\n# TYPE yunnuo_request_duration_seconds histogram")
	fmt.Fprintf(w, "# TYPE yunnuo_disk_free_bytes gauge\nyunnuo_disk_free_bytes %d\n", status.DiskFreeBytes)

	if usage, e := api.service.ReleaseStorageUsage(r.Context()); e == nil {
		u := usage["usage"].(store.ReleaseUsage)
		fmt.Fprintf(w, "# TYPE yunnuo_release_storage_bytes gauge\nyunnuo_release_storage_bytes %d\n# TYPE yunnuo_release_storage_limit_bytes gauge\nyunnuo_release_storage_limit_bytes %d\n", u.TotalBytes, api.service.ResourceLimits().TotalBytes)
	}
	if queue, e := api.service.NotificationStatus(r.Context()); e == nil {
		fmt.Fprintln(w, "# TYPE yunnuo_notifications gauge")
		for _, state := range []string{"pending", "retry", "sending", "sent", "dead"} {
			fmt.Fprintf(w, "yunnuo_notifications{status=%q} %d\n", state, queue[state])
		}
	}
	for i, name := range operationNames {
		m := &api.metrics[i]
		for j, bound := range latencyBounds {
			fmt.Fprintf(w, "yunnuo_request_duration_seconds_bucket{operation=%q,le=%q} %d\n", name, fmt.Sprint(bound), m.buckets[j].Load())
		}
		fmt.Fprintf(w, "yunnuo_request_duration_seconds_bucket{operation=%q,le=\"+Inf\"} %d\nyunnuo_request_duration_seconds_count{operation=%q} %d\n", name, m.requests.Load(), name, m.requests.Load())
		fmt.Fprintf(w, "yunnuo_requests_total{operation=%q} %d\nyunnuo_errors_total{operation=%q} %d\nyunnuo_denials_total{operation=%q} %d\nyunnuo_request_duration_seconds_sum{operation=%q} %.9f\n", name, m.requests.Load(), name, m.errors.Load(), name, m.denials.Load(), name, float64(m.nanos.Load())/1e9)
	}
}

func (api *API) registerOperations(mux *http.ServeMux) {
	mux.HandleFunc("POST /admin/operations/notifications/retry", api.withAdmin(func(w http.ResponseWriter, r *http.Request) {
		if err := api.service.RetryDeadNotifications(r.Context(), adminSessionFrom(r.Context()).UserID); err != nil {
			writeError(w, r, err)
			return
		}
		writeOK(w, r, map[string]bool{"queued": true})
	}, adminSuperRoles...))

	mux.HandleFunc("GET /admin/operations", api.withAdmin(func(w http.ResponseWriter, r *http.Request) {
		storage, err := api.service.ReleaseStorageUsage(r.Context())
		if err != nil {
			writeError(w, r, err)
			return
		}
		metrics := []map[string]any{}
		for i, name := range operationNames {
			m := &api.metrics[i]
			metrics = append(metrics, map[string]any{"operation": name, "requests": m.requests.Load(), "errors": m.errors.Load(), "denials": m.denials.Load(), "duration_seconds": float64(m.nanos.Load()) / 1e9})
		}
		queue, err := api.service.NotificationStatus(r.Context())
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeOK(w, r, map[string]any{"readiness": api.service.Readiness(r.Context()), "storage": storage, "metrics": metrics, "external_metrics_enabled": api.metricsToken != "", "notifications": queue})
	}, adminReadRoles...))
}
