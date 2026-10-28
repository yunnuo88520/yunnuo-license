package httpapi

import (
	core "github.com/yunnuo88520/yunnuo-license/backend/internal/licensing"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestV2BlockAnnouncementIsEscapedAndUnsafeContactURLsAreRejected(t *testing.T) {
	h, token := emailTestHandler(t)
	var app core.CreatedApplication
	decodeData(t, performJSON(t, h, "POST", "/v2/console/applications", token, map[string]any{"name": "Ebox", "block_policy": map[string]any{"enabled": true, "announcement": "站点未授权，联系QQ531023977 <script>alert(1)</script>", "support_url": "https://example.com/contact"}}), &app)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v2/applications/"+app.AppID+"/block-page", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "QQ531023977") || strings.Contains(w.Body.String(), "<script>") || !strings.Contains(w.Body.String(), "&lt;script&gt;") {
		t.Fatal("unescaped announcement or missing notice", w.Code)
	}
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatal("missing block-page CSP")
	}
	performJSONStatus(t, h, "POST", "/v2/console/applications", token, map[string]any{"name": "unsafe", "block_policy": map[string]any{"enabled": true, "support_url": "javascript:alert(1)"}}, 400)
}
