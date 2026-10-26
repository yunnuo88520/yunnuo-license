package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
)

func TestPasswordChangeFailurePreservesSessionsAndSuccessRevokesThem(t *testing.T) {
	for _, portal := range []string{"admin", "developer", "user"} {
		t.Run(portal, func(t *testing.T) {
			ctx := context.Background()
			svc := newHTTPTestService(t)
			root, _, err := svc.EnsureBootstrapAdmin(ctx, "root", "root-password", "Root")
			if err != nil {
				t.Fatal(err)
			}
			const username, original = "password-user", "original-password"
			if portal == "user" {
				_, err = svc.CreateCustomer(ctx, service.CustomerInput{Username: username, Password: original}, root.ID)
			} else {
				_, err = svc.CreateAdminUser(ctx, service.CreateAdminUserInput{Username: username, Password: original, Role: portal})
			}
			if err != nil {
				t.Fatal(err)
			}
			h := New(svc, "", "", "").Handler()
			prefix := "/" + portal
			login := func(password string, status int) string {
				t.Helper()
				recorder := performJSONRequest(t, h, "POST", prefix+"/login", "", map[string]any{"username": username, "password": password})
				if recorder.Code != status {
					t.Fatalf("login: %d, want %d", recorder.Code, status)
				}
				if status != 200 {
					return ""
				}
				var out struct {
					AccessToken string `json:"access_token"`
				}
				decodeData(t, recorder.Body.Bytes(), &out)
				return out.AccessToken
			}
			first, second := login(original, 200), login(original, 200)
			const next = " next-password-123 "
			for _, tc := range []struct{ current, password, confirm, code string }{
				{"wrong-current", next, next, "CURRENT_PASSWORD_INCORRECT"},
				{original, next, "different-password", "PASSWORD_CONFIRMATION_MISMATCH"},
				{original, "short", "short", "NEW_PASSWORD_INVALID"},
				{original, strings.Repeat("x", 257), strings.Repeat("x", 257), "NEW_PASSWORD_INVALID"},
			} {
				recorder := performJSONRequest(t, h, "POST", prefix+"/password", first, map[string]any{"current_password": tc.current, "new_password": tc.password, "confirm_password": tc.confirm})
				if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"`+tc.code+`"`) {
					t.Fatalf("wrong password/confirmation should be a form error: %d %s", recorder.Code, recorder.Body.String())
				}
				for _, token := range []string{first, second} {
					performJSONStatus(t, h, "GET", prefix+"/profile", token, nil, 200)
				}
			}
			login(original, 200)
			performJSONStatus(t, h, "POST", prefix+"/password", "invalid-token", map[string]any{"current_password": original, "new_password": next, "confirm_password": next}, 401)
			performJSON(t, h, "POST", prefix+"/password", first, map[string]any{"current_password": original, "new_password": next, "confirm_password": next})
			for _, token := range []string{first, second} {
				performJSONStatus(t, h, "GET", prefix+"/profile", token, nil, 401)
			}
			login(original, 401)
			login(next, 200)
			login(strings.TrimSpace(next), 401)
		})
	}
}
