package httpapi

import (
	"context"
	"encoding/json"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	core "github.com/yunnuo88520/yunnuo-license/backend/internal/licensing"
)

// Opt-in acceptance runs the exact files returned by the page generator in
// all five real language runtimes against production HTTP handlers.
func TestGeneratedStandardClients(t *testing.T) {
	if os.Getenv("YN_RUN_GENERATED_CLIENTS") != "1" {
		t.Skip("set YN_RUN_GENERATED_CLIENTS=1 with Go, Java, Python, Rust and Node installed")
	}
	svc := newHTTPTestService(t)
	if _, _, e := svc.EnsureBootstrapAdmin(context.Background(), "root", "secret123", "Root"); e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(New(svc, "", "", "").WithAuthorizationV2Only().Handler())
	defer server.Close()
	s := svc.Licensing()
	root := core.BindPrincipal(context.Background(), core.Principal{Actor: core.Actor{Kind: "admin", ID: "test-root", Role: "super_admin"}})
	app, e := s.CreateApplication(root, core.CreateApplicationInput{Name: "Actual five-language product", AllowedModels: []string{"perpetual"}, AllowedBindings: []string{"installation"}, Forms: []string{"gui", "container", "web", "cli", "service"}})
	if e != nil {
		t.Fatal(e)
	}
	plan, e := s.SavePlan(root, app.AppID, "", core.PlanInput{Name: "Standard distribution", Terms: core.Terms{Model: "perpetual", BindingTypes: []string{"installation"}, UpdateDurationDays: 365}, Currency: "CNY"})
	if e != nil {
		t.Fatal(e)
	}
	report := map[string]any{"app_id": app.AppID, "clients": []any{}}
	for _, language := range []string{"go", "java", "python", "rust", "typescript"} {
		t.Run(language, func(t *testing.T) {
			kit, e := s.Integration(root, app.AppID, language)
			if e != nil {
				t.Fatal(e)
			}
			dir := t.TempDir()
			for _, file := range kit.Files {
				if e = os.WriteFile(filepath.Join(dir, file.Name), []byte(file.Content), 0600); e != nil {
					t.Fatal(e)
				}
			}
			sale, e := s.CreateSale(root, app.AppID, core.SaleInput{PlanID: plan.ID, Quantity: 1, DistributeCards: true, Reason: "acceptance paid"}, true)
			if e != nil {
				t.Fatal(e)
			}
			environment := append(os.Environ(), "YN_LICENSE_URL="+server.URL, "YN_STATE_DIR="+filepath.Join(dir, "state"), "YN_CARD="+sale.Cards[0].Code, "YN_BINDING_TYPE=installation", "YN_FORM=gui", "GOFLAGS=-mod=mod")
			var command []string
			switch language {
			case "go":
				runGenerated(t, dir, environment, []string{"go", "mod", "tidy"})
				command = []string{"go", "run", "."}
			case "java":
				runGenerated(t, dir, environment, []string{"mvn", "-q", "compile"})
				command = []string{"mvn", "-q", "exec:java"}
			case "python":
				runGenerated(t, dir, environment, []string{"python3", "-m", "pip", "install", "--quiet", "--target", filepath.Join(dir, "python-deps"), "-r", "requirements.txt"})
				environment = append(environment, "PYTHONPATH="+filepath.Join(dir, "python-deps"))
				command = []string{"python3", "client.py"}
			case "rust":
				runGenerated(t, dir, environment, []string{"cargo", "build", "--release"})
				command = []string{"cargo", "run", "--quiet", "--release", "--"}
			case "typescript":
				runGenerated(t, dir, environment, []string{"npm", "install", "--no-audit", "--no-fund"})
				runGenerated(t, dir, environment, []string{"npm", "run", "check"})
				command = []string{"node", "client.ts"}
			}
			for _, operation := range []string{"activate", "verify", "local"} {
				args := append([]string{}, command...)
				if language == "java" {
					args = append(args, "-Dexec.args="+operation)
				} else {
					args = append(args, operation)
				}
				output := runGenerated(t, dir, environment, args)
				if !strings.Contains(output, "\"valid\":true") && !strings.Contains(output, "\"valid\": true") {
					t.Fatalf("%s did not authorize: %s", operation, output)
				}
			}
			// A refusal must clear tokens while retaining installation identity;
			// renewal/restoration and voluntary release must not require a card.
			noCard := append(append([]string{}, environment...), "YN_CARD=")
			operationArgs := func(operation string) []string {
				args := append([]string{}, command...)
				if language == "java" {
					return append(args, "-Dexec.args="+operation)
				}
				return append(args, operation)
			}
			if e = s.SetLicenseStatus(root, app.AppID, sale.Cards[0].LicenseID, "suspended", "pause test"); e != nil {
				t.Fatal(e)
			}
			suspended := operationArgs("verify")
			denied := exec.Command(suspended[0], suspended[1:]...)
			denied.Dir = dir
			denied.Env = noCard
			if raw, e := denied.CombinedOutput(); e == nil || !strings.Contains(string(raw), "LICENSE_SUSPENDED") {
				t.Fatalf("suspension did not reject: %v %s", e, raw)
			}
			if e = s.SetLicenseStatus(root, app.AppID, sale.Cards[0].LicenseID, "active", "restore test"); e != nil {
				t.Fatal(e)
			}
			runGenerated(t, dir, noCard, operationArgs("verify"))
			runGenerated(t, dir, noCard, operationArgs("release"))
			runGenerated(t, dir, noCard, operationArgs("verify"))
			// Cached local verification must work with the network unavailable.
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			offlineURL := "http://" + listener.Addr().String()
			listener.Close()
			offlineEnv := append(append([]string{}, environment...), "YN_LICENSE_URL="+offlineURL)
			for _, operation := range []string{"local", "verify"} {
				args := append([]string{}, command...)
				if language == "java" {
					args = append(args, "-Dexec.args="+operation)
				} else {
					args = append(args, operation)
				}
				output := runGenerated(t, dir, offlineEnv, args)
				if operation == "verify" && !strings.Contains(output, "\"offline\":true") && !strings.Contains(output, "\"offline\": true") {
					t.Fatalf("network failure did not use signed offline grant: %s", output)
				}
			}
			if _, e = s.RotateSigningKey(root, app.AppID, "generated client rotation acceptance", false); e != nil {
				t.Fatal(e)
			}
			refreshArgs := append([]string{}, command...)
			if language == "java" {
				refreshArgs = append(refreshArgs, "-Dexec.args=verify")
			} else {
				refreshArgs = append(refreshArgs, "verify")
			}
			runGenerated(t, dir, environment, refreshArgs)
			// A different customer uses the exact same generated program and public
			// AppID, changing only card and private state directory.
			second, e := s.CreateSale(root, app.AppID, core.SaleInput{PlanID: plan.ID, Quantity: 1, DistributeCards: true, Reason: "second customer"}, true)
			if e != nil {
				t.Fatal(e)
			}
			otherEnv := append(append([]string{}, environment...), "YN_STATE_DIR="+filepath.Join(dir, "customer-two"), "YN_CARD="+second.Cards[0].Code)
			args := append([]string{}, command...)
			if language == "java" {
				args = append(args, "-Dexec.args=activate")
			} else {
				args = append(args, "activate")
			}
			runGenerated(t, dir, otherEnv, args)
			// An explicit server revocation must clear cached tokens, never use the
			// separately signed offline allowance to override the rejection.
			if e = s.SetLicenseStatus(root, app.AppID, sale.Cards[0].LicenseID, "revoked", "acceptance revoke"); e != nil {
				t.Fatal(e)
			}
			args = append([]string{}, command...)
			if language == "java" {
				args = append(args, "-Dexec.args=verify")
			} else {
				args = append(args, "verify")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, args[0], args[1:]...)
			cmd.Dir = dir
			cmd.Env = environment
			if output, e := cmd.CombinedOutput(); e == nil || !strings.Contains(string(output), "LICENSE_REVOKED") {
				t.Fatalf("revoked license not rejected: %v %s", e, output)
			}
			raw, e := os.ReadFile(filepath.Join(dir, "state", app.AppID, "cache.json"))
			if e != nil {
				t.Fatal(e)
			}
			var cache map[string]any
			if e = json.Unmarshal(raw, &cache); e != nil {
				t.Fatal(e)
			}
			if token, _ := cache["jwt"].(string); token != "" {
				t.Fatal("rejected token retained")
			}
			report["clients"] = append(report["clients"].([]any), map[string]any{"language": language, "activation": true, "refresh": true, "local_verification": true, "same_package_two_customers": true, "revocation_clears_cache": true, "resume_without_card": true, "release_and_reacquire_without_card": true, "network_only_offline_grant": true, "cached_local_verification": true, "jwks_rotation_without_rebuild": true})
		})
	}
	if output := os.Getenv("YN_V2_CLIENT_REPORT"); output != "" {
		raw, _ := json.MarshalIndent(report, "", "  ")
		if e = os.WriteFile(output, raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
}
func runGenerated(t *testing.T, dir string, environment, args []string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = environment
	raw, e := cmd.CombinedOutput()
	if e != nil {
		if len(raw) > 6000 {
			raw = raw[len(raw)-6000:]
		}
		t.Fatalf("generated %s failed: %v\n%s", args[0], e, raw)
	}
	return string(raw)
}
