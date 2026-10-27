// A standard product client: customer secrets and identity live only in dataDir.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	yn "github.com/yunnuo88520/yunnuo-license/sdk/go"
	"html/template"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

var bundledVersion string
var healthFailure string

type config struct {
	BaseURL        string            `json:"base_url"`
	AppKey         string            `json:"app_key"`
	ProductID      string            `json:"product_id"`
	ProductCode    string            `json:"product_code"`
	BindMode       string            `json:"bind_mode"`
	BindValue      string            `json:"bind_value"`
	PublicKeys     map[int]string    `json:"public_keys"`
	CurrentVersion string            `json:"current_version"`
	Target         *yn.ReleaseTarget `json:"target,omitempty"`
	DeviceID       string            `json:"device_id,omitempty"`
}
type state struct {
	LegacyAttach bool               `json:"legacy_attach,omitempty"`
	License      yn.LicenseResponse `json:"license"`
	BindMode     string             `json:"bind_mode"`
	Registration string             `json:"registration"`
	Seat         *yn.SeatLease      `json:"seat,omitempty"`
}
type app struct {
	config                       config
	client                       *yn.Client
	identity                     yn.ClientInstance
	state                        state
	dir, host, actualIP, session string
	mutex                        sync.Mutex
}

func fail(code string) error { return &yn.APIError{Code: code, Message: code, HTTPStatus: 403} }
func (a *app) save() error {
	raw, e := json.MarshalIndent(a.state, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(a.dir, ".state-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(raw); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(f.Name(), filepath.Join(a.dir, "license.json"))
}
func (a *app) input(r *http.Request) (yn.VerifyRequest, error) {
	if a.state.License.LicenseNo == "" {
		return yn.VerifyRequest{}, fail("LICENSE_REQUIRED")
	}
	instance := a.identity
	if a.state.LegacyAttach {
		instance = yn.ClientInstance{}
	}
	instance.SessionID = a.session
	instance.Target = a.config.Target
	instance.CurrentVersion = a.config.CurrentVersion
	if a.state.Seat != nil {
		instance.SeatID = a.state.Seat.ID
	}
	value := a.state.Registration
	mode := a.state.BindMode
	if mode == "instance" {
		value = instance.InstanceID
	}
	if r == nil && a.host != "" && (mode == "domain" || mode == "domain_wildcard") {
		value = a.host
	}
	if r == nil && a.actualIP != "" && mode == "ip_cidr" {
		value = a.actualIP
	}
	in := yn.VerifyRequest{ClientInstance: instance, LicenseNo: a.state.License.LicenseNo, BindMode: mode, BindValue: value}
	if r == nil && a.host != "" && mode == "composite" {
		return yn.BindingForHTTPRequest(in, &http.Request{Host: a.host}, nil)
	}
	if r != nil {
		return yn.BindingForHTTPRequest(in, r, nil)
	}
	return in, nil
}
func (a *app) verify(ctx context.Context, r *http.Request) (yn.LicenseResponse, error) {
	in, e := a.input(r)
	if e != nil {
		return yn.LicenseResponse{}, e
	}
	out, e := a.client.Verify(ctx, in)
	if yn.IsAPIErrorCode(e, "SEAT_REQUIRED") {
		seat, se := a.client.AcquireSeat(ctx, yn.SeatRequest{VerifyRequest: in, SessionID: a.session})
		if se != nil {
			return out, se
		}
		a.state.Seat = &seat
		in.SeatID = seat.ID
		out, e = a.client.Verify(ctx, in)
	}
	if e != nil {
		return out, e
	}
	if !out.CanUse {
		return out, fail("USAGE_DENIED")
	}
	a.state.License = out
	if e = a.save(); e != nil {
		return out, e
	}
	return out, nil
}
func randomID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func main() {
	if e := run(); e != nil {
		var api *yn.APIError
		code := "CLIENT_ERROR"
		if errors.As(e, &api) {
			code = api.Code
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"success": false, "code": code, "message": e.Error()})
		os.Exit(1)
	}
}
func run() error {
	flags := flag.NewFlagSet("product-client", flag.ContinueOnError)
	cfgPath := flags.String("config", "config.json", "public product config")
	dataDir := flags.String("data", ".license-data", "private persistent state")
	host := flags.String("host", "", "actual host for a command-line check")
	ip := flags.String("ip", "", "actual protected IP")
	session := flags.String("session", "", "process/session identity")
	if e := flags.Parse(os.Args[1:]); e != nil {
		return e
	}
	args := flags.Args()
	command := "verify"
	value := ""
	if len(args) > 0 {
		command = args[0]
	}
	if len(args) > 1 {
		value = args[1]
	}
	raw, e := os.ReadFile(*cfgPath)
	if e != nil {
		return e
	}
	a := &app{dir: *dataDir, host: *host, actualIP: *ip, session: *session}
	if e = json.Unmarshal(raw, &a.config); e != nil {
		return e
	}
	if bundledVersion != "" {
		a.config.CurrentVersion = bundledVersion
	}
	if a.config.CurrentVersion == "" {
		a.config.CurrentVersion = "1.0.0"
	}
	if a.config.ProductID == "" {
		a.config.ProductID = a.config.AppKey
	}
	if *dataDir == ".license-data" {
		namespace := sha256.Sum256([]byte(a.config.AppKey))
		a.dir = filepath.Join(*dataDir, hex.EncodeToString(namespace[:8]))
	}
	a.identity, e = yn.OpenInstallation(a.dir, a.config.DeviceID)
	if e != nil {
		return e
	}
	a.client, e = yn.NewClient(a.config.BaseURL, a.config.AppKey, yn.WithHTTPClient(&http.Client{Timeout: 120 * time.Second}))
	if e != nil {
		return e
	}
	if raw, e = os.ReadFile(filepath.Join(a.dir, "license.json")); e == nil {
		if e = json.Unmarshal(raw, &a.state); e != nil {
			return e
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	if a.session == "" {
		a.session = randomID()
	}
	if a.state.Seat != nil && a.state.Seat.Borrowed {
		a.session = a.state.Seat.SessionID
	}
	if value != "" && (command == "verify" || command == "heartbeat" || command == "claim") {
		if a.state.BindMode == "ip_cidr" {
			a.actualIP = value
		} else {
			a.host = value
		}
	}
	ctx := context.Background()
	defer func() {
		if *session == "" && command != "serve" && a.state.Seat != nil && !a.state.Seat.Borrowed {
			if in, e := a.input(nil); e == nil {
				_ = a.client.ReleaseSeat(context.Background(), in)
			}
		}
	}()
	output := func(v any) error { return json.NewEncoder(os.Stdout).Encode(v) }
	summary := func(v yn.LicenseResponse) error {
		return output(map[string]any{"success": true, "license_no": v.LicenseNo, "can_use": v.CanUse, "can_update": v.CanUpdate, "usage_expired_at": v.UsageExpiredAt, "updates_expired_at": v.UpdatesExpiredAt, "authorization_terms": v.AuthorizationTerms, "version": a.config.CurrentVersion, "instance_id": a.identity.InstanceID})
	}
	switch command {
	case "note", "read-note":
		if _, e := a.verify(ctx, nil); e != nil {
			return e
		}
		path := filepath.Join(a.dir, "customer-note.txt")
		if command == "note" {
			if e = os.WriteFile(path, []byte(value), 0600); e != nil {
				return e
			}
			return output(map[string]bool{"saved": true})
		}
		bytes, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		return output(map[string]string{"note": string(bytes)})

	case "machine-code":
		return output(map[string]string{"device_id": a.identity.DeviceID})
	case "verify-package":
		if len(args) < 3 {
			return errors.New("release JSON and package path required")
		}
		raw, e := os.ReadFile(value)
		if e != nil {
			return e
		}
		var r yn.ProductRelease
		if e = json.Unmarshal(raw, &r); e != nil {
			return e
		}
		data, e := os.ReadFile(args[2])
		if e != nil {
			return e
		}
		if e = yn.VerifyReleasePackage(r, data, yn.ReleaseExpectation{PublicKeyPEM: a.config.PublicKeys[r.KeyVersion], ProductID: a.config.ProductID, Target: a.config.Target, CurrentVersion: a.config.CurrentVersion}); e != nil {
			return e
		}
		return output(r)
	case "activate":
		if value == "" {
			return errors.New("card required")
		}
		req, e := a.client.CardRequirements(ctx, value)
		if e != nil {
			return e
		}
		terms, ok := req["authorization_terms"].(map[string]any)
		if !ok {
			return errors.New("invalid card requirements")
		}
		mode, _ := terms["bind_mode"].(string)
		registration := a.config.BindValue
		if env, ok := os.LookupEnv("LICENSE_BIND_VALUE"); ok {
			registration = env
		}
		if len(args) > 2 {
			registration = args[2]
		}
		if mode == "device" && registration == "" {
			registration = a.identity.DeviceID
		}
		if mode == "instance" {
			registration = a.identity.InstanceID
		}
		if mode == "none" {
			registration = "none"
		}
		issued, e := a.client.Activate(ctx, yn.ActivateRequest{ClientInstance: a.identity, CardCode: value, BindMode: mode, BindValue: registration, DeviceName: "standard-product-client", ClientVersion: a.config.CurrentVersion})
		if e != nil {
			return e
		}
		if !issued.CanUse {
			return fail("USAGE_DENIED")
		}
		a.state = state{License: issued, BindMode: mode, Registration: issued.BindValue}
		if e = a.save(); e != nil {
			return e
		}
		return summary(issued)
	case "attach":
		if value == "" {
			return errors.New("license number required")
		}
		a.state = state{LegacyAttach: true, License: yn.LicenseResponse{LicenseNo: value}, BindMode: a.config.BindMode, Registration: a.config.BindValue}
		result, e := a.verify(ctx, nil)
		if e != nil {
			return e
		}
		return summary(result)
	case "verify":
		if healthFailure == "true" {
			return errors.New("new product health check failed")
		}
		result, e := a.verify(ctx, nil)
		if e != nil {
			return e
		}
		return summary(result)
	case "heartbeat":
		if _, e := a.verify(ctx, nil); e != nil {
			return e
		}
		in, e := a.input(nil)
		if e != nil {
			return e
		}
		out, e := a.client.Heartbeat(ctx, yn.HeartbeatRequest{ClientInstance: in.ClientInstance, LicenseNo: in.LicenseNo, BindMode: in.BindMode, BindValue: in.BindValue})
		if e != nil {
			return e
		}
		return output(out)
	case "renew":
		in, e := a.input(nil)
		if e != nil {
			return e
		}
		out, e := a.client.Renew(ctx, yn.RenewRequest{ClientInstance: in.ClientInstance, LicenseNo: in.LicenseNo, BindMode: in.BindMode, BindValue: in.BindValue, RenewCardCode: value})
		if e != nil {
			return e
		}
		a.state.License = out
		if e = a.save(); e != nil {
			return e
		}
		return summary(out)
	case "claim":
		in, e := a.input(nil)
		if e != nil {
			return e
		}
		out, e := a.client.CreateClaimTicket(ctx, in)
		if e != nil {
			return e
		}
		return output(out)
	case "borrow":
		days := 1
		if value != "" {
			if _, e = fmt.Sscan(value, &days); e != nil {
				return e
			}
		}
		in, e := a.input(nil)
		if e != nil {
			return e
		}
		out, e := a.client.AcquireSeat(ctx, yn.SeatRequest{VerifyRequest: in, SessionID: a.session, BorrowDays: days})
		if e != nil {
			return e
		}
		a.state.Seat = &out
		if e = a.save(); e != nil {
			return e
		}
		return output(map[string]any{"borrowed": out.Borrowed, "expires_at": out.ExpiresAt})
	case "release":
		in, e := a.input(nil)
		if e != nil {
			return e
		}
		if e = a.client.ReleaseSeat(ctx, in); e != nil {
			return e
		}
		if a.state.Seat != nil && !a.state.Seat.Borrowed {
			a.state.Seat = nil
		}
		if e = a.save(); e != nil {
			return e
		}
		return output(map[string]bool{"accepted": true})
	case "verify-offline":
		in, e := a.input(nil)
		if e != nil {
			return e
		}
		verifier, e := yn.NewKeyRingOfflineVerifier(a.config.PublicKeys, a.config.ProductCode)
		if e != nil {
			return e
		}
		token := a.state.License.OfflineToken
		if a.state.Seat != nil && a.state.Seat.Borrowed {
			token = a.state.Seat.Token
		}
		guard, e := yn.NewHighWaterGuard(fileClock{filepath.Join(a.dir, "clock.json")}, time.Second)
		if e != nil {
			return e
		}
		if e = guard.CheckAndUpdate(ctx, time.Now()); e != nil {
			return e
		}
		expected := yn.OfflineExpectation{LicenseNo: in.LicenseNo, BindMode: in.BindMode, BindValue: in.BindValue, InstanceID: a.identity.InstanceID}
		if a.state.Seat != nil && a.state.Seat.Borrowed {
			expected.SeatID = a.state.Seat.ID
			expected.SessionID = a.state.Seat.SessionID
		}
		claims, e := verifier.VerifyOfflineToken(token, expected)
		if e != nil {
			return e
		}
		if !claims.CanUse {
			return fail("USAGE_DENIED")
		}
		return output(map[string]any{"can_use": true, "can_update": false, "offline": true, "authorization_terms": claims.AuthorizationTerms})
	case "offline-file", "import-offline", "verify-file":
		if command == "verify-file" {
			value = filepath.Join(a.dir, "offline.key")
		}
		raw, e := os.ReadFile(value)
		if e != nil {
			return e
		}
		verifier, e := yn.NewKeyRingOfflineVerifier(a.config.PublicKeys, a.config.ProductCode)
		if e != nil {
			return e
		}
		guard, e := yn.NewHighWaterGuard(fileClock{filepath.Join(a.dir, "clock.json")}, time.Second)
		if e != nil {
			return e
		}
		if e = guard.CheckAndUpdate(ctx, time.Now()); e != nil {
			return e
		}
		claims, e := verifier.VerifyLicenseFile(raw, a.identity.DeviceID)
		if e != nil {
			return e
		}
		if command == "import-offline" {
			f, e := os.CreateTemp(a.dir, ".offline-")
			if e != nil {
				return e
			}
			defer os.Remove(f.Name())
			if _, e = f.Write(raw); e != nil {
				f.Close()
				return e
			}
			if e = f.Close(); e != nil {
				return e
			}
			if e = os.Rename(f.Name(), filepath.Join(a.dir, "offline.key")); e != nil {
				return e
			}
		}
		return output(map[string]any{"can_use": true, "can_update": false, "offline": true, "license_no": claims.LicenseNo, "usage_expired_at": claims.ExpiredAt, "version": a.config.CurrentVersion})
	case "updates", "download":
		if value != "" {
			a.config.CurrentVersion = value
		}
		in, e := a.input(nil)
		if e != nil {
			return e
		}
		out, e := a.client.CheckUpdate(ctx, yn.CheckUpdateRequest{VerifyRequest: in, CurrentVersion: a.config.CurrentVersion})
		if e != nil {
			return e
		}
		if command == "download" && out.Release != nil && out.UpdateAvailable {
			release := *out.Release
			key := a.config.PublicKeys[release.KeyVersion]
			if key == "" {
				return errors.New("unknown signing key version")
			}
			bytes, e := a.client.DownloadUpdate(ctx, release, in, yn.ReleaseExpectation{PublicKeyPEM: key, ProductID: a.config.ProductID, Target: a.config.Target, CurrentVersion: a.config.CurrentVersion})
			if e != nil {
				return e
			}
			path := filepath.Join(a.dir, "update.package")
			if e = os.WriteFile(path, bytes, 0600); e != nil {
				return e
			}
			sum := sha256.Sum256(bytes)
			return output(map[string]any{"downloaded": true, "path": path, "sha256": hex.EncodeToString(sum[:]), "release": release})
		}
		return output(out)
	case "authorize":
		in, e := a.input(nil)
		if e != nil {
			return e
		}
		if e = a.client.AuthorizeUpdate(ctx, value, in); e != nil {
			return e
		}
		return output(map[string]bool{"authorized": true})
	case "serve":
		port := os.Getenv("DEMO_PORT")
		if port == "" {
			port = "3000"
		}
		address := net.JoinHostPort("127.0.0.1", port)
		if bind := os.Getenv("DEMO_ADDR"); bind != "" {
			address = bind
		}
		server := &http.Server{Addr: address, ReadHeaderTimeout: 5 * time.Second}
		server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			if r.URL.Path == "/healthz" {
				if healthFailure == "true" {
					w.WriteHeader(503)
					return
				}
				w.WriteHeader(200)
				return
			}
			if r.URL.Path != "/protected" && r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			a.mutex.Lock()
			defer a.mutex.Unlock()
			rights, e := a.verify(r.Context(), r)
			if e != nil {
				code := 503
				var api *yn.APIError
				if errors.As(e, &api) && api.HTTPStatus < 500 {
					code = 403
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(code)
				json.NewEncoder(w).Encode(map[string]any{"can_use": false, "error": e.Error()})
				return
			}
			if r.URL.Path == "/protected" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"can_use": rights.CanUse, "can_update": rights.CanUpdate, "version": a.config.CurrentVersion, "instance_id": a.identity.InstanceID, "note": func() string { v, _ := os.ReadFile(filepath.Join(a.dir, "customer-note.txt")); return string(v) }()})
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			template.Must(template.New("page").Parse(siteHTML)).Execute(w, map[string]any{"Version": a.config.CurrentVersion, "CanUpdate": rights.CanUpdate})
		})
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(stop)
		go func() { <-stop; _ = server.Shutdown(context.Background()) }()
		fmt.Println("http://" + address + "/protected")
		e = server.ListenAndServe()
		if errors.Is(e, http.ErrServerClosed) {
			if in, ie := a.input(nil); ie == nil {
				_ = a.client.ReleaseSeat(ctx, in)
			}
			return nil
		}
		return e
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
}

type fileClock struct{ path string }

func (f fileClock) Load(ctx context.Context) (time.Time, error) {
	raw, e := os.ReadFile(f.path)
	if os.IsNotExist(e) {
		return time.Time{}, nil
	}
	if e != nil {
		return time.Time{}, e
	}
	var t time.Time
	e = json.Unmarshal(raw, &t)
	return t, e
}
func (f fileClock) Save(ctx context.Context, t time.Time) error {
	raw, e := json.Marshal(t)
	if e != nil {
		return e
	}
	return os.WriteFile(f.path, raw, 0600)
}

const siteHTML = `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Ebox · 标准产品</title><style>body{margin:0;background:#f2f5f1;color:#173b2a;font:17px/1.8 system-ui;display:grid;place-items:center;min-height:100vh}main{margin:24px;padding:48px;background:white;border:1px solid #d6e1d7;border-radius:24px;max-width:760px}h1{font-size:38px;line-height:1.3}small{color:#687b6d}.badge{display:inline-block;padding:5px 12px;background:#e7f3e9;border-radius:8px}</style><main><small>EBOX / 标准安装包交付</small><h1>网站已经通过授权校验</h1><p>此页面使用实际访问域名验证。每位客户使用同一份程序，分别持有自己的授权和安装身份。</p><span class="badge">v{{.Version}}</span><p>{{if .CanUpdate}}更新服务有效{{else}}更新服务到期，当前版本继续使用{{end}}</p></main></html>`
