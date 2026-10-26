package httpapi

import (
	"bufio"
	"context"
	"encoding/base64"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
)

// fakeSMTP 最小的可编程 SMTP 服务器，按投递捕获全部报文。
type fakeSMTP struct {
	listener   net.Listener
	mu         sync.Mutex
	authed     bool
	deliveries []smtpDelivery
}

type smtpDelivery struct {
	mailFrom string
	rcptTo   []string
	data     string
}

func startFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &fakeSMTP{listener: ln}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return s
}

func (s *fakeSMTP) port() int {
	return s.listener.Addr().(*net.TCPAddr).Port
}

// findDelivery 返回解码后正文包含 marker 的那封邮件。
func (s *fakeSMTP) findDelivery(t *testing.T, marker string) smtpDelivery {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.deliveries {
		if strings.Contains(smtpBodyText(t, d), marker) {
			return d
		}
	}
	t.Fatalf("no delivery contains %q; got %d deliveries", marker, len(s.deliveries))
	return smtpDelivery{}
}

// smtpBodyText 解出报文 base64 正文。
func smtpBodyText(t *testing.T, d smtpDelivery) string {
	t.Helper()
	payload := ""
	for _, line := range strings.Split(strings.TrimSpace(d.data), "\r\n") {
		if strings.Contains(line, ": ") {
			continue
		}
		payload += strings.TrimSpace(line)
	}
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("decode mail body: %v\n%s", err, d.data)
	}
	return string(decoded)
}

func (s *fakeSMTP) serve(conn net.Conn) {
	defer conn.Close()
	write := func(line string) {
		_, _ = conn.Write([]byte(line + "\r\n"))
	}
	write("220 test.local ESMTP")
	br := bufio.NewReader(conn)
	var current smtpDelivery
	dataMode := false
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if dataMode {
			if trimmed == "." {
				dataMode = false
				s.mu.Lock()
				s.deliveries = append(s.deliveries, current)
				s.mu.Unlock()
				current = smtpDelivery{}
				write("250 OK queued")
			} else {
				current.data += line
			}
			continue
		}
		upper := strings.ToUpper(trimmed)
		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			write("250-test.local")
			write("250-AUTH PLAIN")
			write("250 OK")
		case strings.HasPrefix(upper, "AUTH PLAIN"):
			s.mu.Lock()
			s.authed = true
			s.mu.Unlock()
			write("235 OK")
		case strings.HasPrefix(upper, "MAIL FROM:"):
			current.mailFrom = strings.TrimPrefix(trimmed[len("MAIL FROM:"):], "<")
			current.mailFrom = strings.TrimSuffix(current.mailFrom, ">")
			write("250 OK")
		case strings.HasPrefix(upper, "RCPT TO:"):
			rcpt := strings.TrimPrefix(trimmed[len("RCPT TO:"):], "<")
			rcpt = strings.TrimSuffix(rcpt, ">")
			current.rcptTo = append(current.rcptTo, rcpt)
			write("250 OK")
		case upper == "DATA":
			dataMode = true
			write("354 go")
		case upper == "QUIT":
			write("221 bye")
			return
		case upper == "RSET", upper == "NOOP":
			write("250 OK")
		default:
			write("250 OK")
		}
	}
}

func emailTestHandler(t *testing.T) (http.Handler, string) {
	t.Helper()
	ctx := context.Background()
	svc := newHTTPTestService(t)
	if _, _, err := svc.EnsureBootstrapAdmin(ctx, "root", "secret123", "Root Admin"); err != nil {
		t.Fatal(err)
	}
	handler := New(svc, "", "", "").Handler()
	return handler, loginAdmin(t, handler, "root", "secret123")
}

func TestEmailSettingsRoundTripMasksPassword(t *testing.T) {
	handler, token := emailTestHandler(t)

	var settings service.EmailSettingsView
	decodeData(t, performJSON(t, handler, http.MethodGet, "/admin/email-settings", token, nil), &settings)
	if settings.Enabled || settings.HasPassword {
		t.Fatalf("fresh settings should be disabled without password: %#v", settings)
	}

	performJSONStatus(t, handler, http.MethodGet, "/admin/email-settings", "", nil, http.StatusUnauthorized)

	body := performJSON(t, handler, http.MethodPost, "/admin/email-settings", token, map[string]any{
		"enabled": true, "smtp_host": "smtp.example.com", "smtp_port": 465,
		"smtp_username": "noreply@example.com", "password": "s3cret",
		"from_address": "noreply@example.com", "from_name": "测试站点",
		"encryption": "ssl", "site_url": "https://mail.example.com/",
	})
	decodeData(t, body, &settings)
	if !settings.Enabled || settings.SMTPHost != "smtp.example.com" || !settings.HasPassword {
		t.Fatalf("settings not updated: %#v", settings)
	}
	if settings.SiteURL != "https://mail.example.com" {
		t.Fatalf("site URL should be normalized, got %q", settings.SiteURL)
	}
	if strings.Contains(string(body), "s3cret") || strings.Contains(string(body), "smtp_password_enc") {
		t.Fatalf("password must not appear in the response: %s", body)
	}

	// 密码留空 = 保持原值；关闭启用。
	body = performJSON(t, handler, http.MethodPost, "/admin/email-settings", token, map[string]any{
		"enabled": false, "smtp_host": "smtp.example.com", "smtp_port": 587,
		"smtp_username": "noreply@example.com", "password": "",
		"from_address": "noreply@example.com", "from_name": "测试站点",
		"encryption": "starttls", "site_url": "",
	})
	decodeData(t, body, &settings)
	if !settings.HasPassword || settings.Enabled || settings.Encryption != "starttls" {
		t.Fatalf("password should be preserved and disabled applied: %#v", settings)
	}

	// 非法输入。
	performJSONStatus(t, handler, http.MethodPost, "/admin/email-settings", token, map[string]any{
		"enabled": true, "smtp_host": "", "smtp_port": 465, "password": "x",
		"from_address": "noreply@example.com", "encryption": "ssl",
	}, http.StatusBadRequest)
	performJSONStatus(t, handler, http.MethodPost, "/admin/email-settings", token, map[string]any{
		"enabled": true, "smtp_host": "h", "smtp_port": 70000, "password": "x",
		"from_address": "noreply@example.com", "encryption": "ssl",
	}, http.StatusBadRequest)
	performJSONStatus(t, handler, http.MethodPost, "/admin/email-settings", token, map[string]any{
		"enabled": true, "smtp_host": "h", "smtp_port": 465, "password": "x",
		"from_address": "bad-address", "encryption": "ssl",
	}, http.StatusBadRequest)
}

func TestSendTestEmailAgainstLocalSMTP(t *testing.T) {
	smtpSrv := startFakeSMTP(t)
	handler, token := emailTestHandler(t)

	performJSON(t, handler, http.MethodPost, "/admin/email-settings", token, map[string]any{
		"enabled": true, "smtp_host": "localhost", "smtp_port": smtpSrv.port(),
		"from_address": "noreply@example.com", "from_name": "允诺云授权",
		"encryption": "none", "site_url": "http://localhost:8080",
	})
	performJSON(t, handler, http.MethodPost, "/admin/email-settings/test", token, map[string]any{
		"to_address": "agent@example.com",
	})

	d := smtpSrv.findDelivery(t, "测试邮件")
	if len(d.rcptTo) != 1 || d.rcptTo[0] != "agent@example.com" {
		t.Fatalf("unexpected recipients: %v", d.rcptTo)
	}
	if d.mailFrom != "noreply@example.com" {
		t.Fatalf("unexpected mail from: %q", d.mailFrom)
	}
	raw := d.data
	if !strings.Contains(raw, "Subject: =?utf-8?q?") {
		t.Fatalf("subject should be MIME encoded:\n%s", raw)
	}
	body := smtpBodyText(t, d)
	for _, want := range []string{"允诺云授权", "测试邮件"} {
		if !strings.Contains(body, want) {
			t.Errorf("mail body missing %q:\n%s", want, body)
		}
	}

	var logs []map[string]any
	decodeData(t, performJSON(t, handler, http.MethodGet, "/admin/email-logs?limit=10", token, nil), &logs)
	if len(logs) != 1 || logs[0]["status"] != "sent" || logs[0]["kind"] != "test" {
		t.Fatalf("unexpected email logs: %#v", logs)
	}
}

func TestEmailSettingsUsernameDefaultsToFromAddress(t *testing.T) {
	handler, token := emailTestHandler(t)

	// QQ 邮箱等场景：只填授权码不填用户名，应自动用发件人地址作为用户名。
	var settings service.EmailSettingsView
	body := performJSON(t, handler, http.MethodPost, "/admin/email-settings", token, map[string]any{
		"enabled": true, "smtp_host": "smtp.qq.com", "smtp_port": 465,
		"smtp_username": "", "password": "auth-code",
		"from_address": "xinshijie1216@qq.com", "from_name": "允诺云授权",
		"encryption": "ssl",
	})
	decodeData(t, body, &settings)
	if settings.SMTPUsername != "xinshijie1216@qq.com" || !settings.HasPassword || !settings.Enabled {
		t.Fatalf("username should default to from address: %#v", settings)
	}

	// 再次保存时用户名留空 = 保持已保存的用户名（密码同样留空保持）。
	body = performJSON(t, handler, http.MethodPost, "/admin/email-settings", token, map[string]any{
		"enabled": true, "smtp_host": "smtp.qq.com", "smtp_port": 465,
		"smtp_username": "", "password": "",
		"from_address": "xinshijie1216@qq.com", "from_name": "允诺云授权",
		"encryption": "ssl",
	})
	decodeData(t, body, &settings)
	if settings.SMTPUsername != "xinshijie1216@qq.com" || !settings.HasPassword {
		t.Fatalf("saved username should be preserved: %#v", settings)
	}
}

func TestAgentEmailGuards(t *testing.T) {
	handler, token := emailTestHandler(t)

	performJSON(t, handler, http.MethodPost, "/admin/email-settings", token, map[string]any{
		"enabled": true, "smtp_host": "localhost", "smtp_port": 1,
		"from_address": "noreply@example.com", "encryption": "none",
	})
	var agent struct {
		ID string `json:"id"`
	}
	decodeData(t, performJSON(t, handler, http.MethodPost, "/admin/agents", token, map[string]any{
		"name": "无邮箱代理",
	}), &agent)

	performJSONStatus(t, handler, http.MethodPost, "/admin/agents/"+agent.ID+"/email", token, map[string]any{
		"subject": "主题", "body": "正文",
	}, http.StatusBadRequest)

	// 未启用邮件功能时同样拒绝。
	performJSON(t, handler, http.MethodPost, "/admin/email-settings", token, map[string]any{
		"enabled": false, "smtp_host": "localhost", "smtp_port": 1,
		"from_address": "noreply@example.com", "encryption": "none",
	})
	performJSONStatus(t, handler, http.MethodPost, "/admin/email-settings/test", token, map[string]any{
		"to_address": "someone@example.com",
	}, http.StatusBadRequest)
}

func TestRecoveryEmailDeliveredToAgentMailbox(t *testing.T) {
	smtpSrv := startFakeSMTP(t)
	handler, token := emailTestHandler(t)

	performJSON(t, handler, http.MethodPost, "/admin/email-settings", token, map[string]any{
		"enabled": true, "smtp_host": "localhost", "smtp_port": smtpSrv.port(),
		"from_address": "noreply@example.com", "encryption": "none",
		"site_url": "http://localhost:8080",
	})

	var agent struct {
		ID string `json:"id"`
	}
	decodeData(t, performJSON(t, handler, http.MethodPost, "/admin/agents", token, map[string]any{
		"name": "有邮箱代理", "email": "Agent-Mail@Example.com",
	}), &agent)
	var user struct {
		ID string `json:"id"`
	}
	decodeData(t, performJSON(t, handler, http.MethodPost, "/admin/agents/"+agent.ID+"/users", token, map[string]any{
		"username": "agentuser", "password": "secret123", "display_name": "账号 A", "role": "agent_owner", "email": "agent-mail@example.com",
	}), &user)

	var accounts []map[string]any
	decodeData(t, performJSON(t, handler, http.MethodGet, "/admin/recovery/accounts?kind=agent&agent_id="+agent.ID, token, nil), &accounts)
	if len(accounts) != 1 || accounts[0]["email"] != "agent-mail@example.com" {
		t.Fatalf("recovery accounts should expose email: %#v", accounts)
	}

	body := performJSON(t, handler, http.MethodPost, "/admin/recovery/agent/"+user.ID+"/email", token, map[string]any{
		"reason": "账号持有人忘记密码", "minutes": 30,
	})
	var result struct {
		SentTo string `json:"sent_to"`
	}
	decodeData(t, body, &result)
	if result.SentTo != "agent-mail@example.com" {
		t.Fatalf("unexpected sent_to: %q", result.SentTo)
	}
	if strings.Contains(string(body), "reset_") {
		t.Fatalf("recovery code must not be returned by email endpoint: %s", body)
	}
	d := smtpSrv.findDelivery(t, "密码重置验证码")
	if len(d.rcptTo) != 1 || d.rcptTo[0] != "agent-mail@example.com" {
		t.Fatalf("unexpected recipients: %v", d.rcptTo)
	}
	mailBody := smtpBodyText(t, d)
	if !strings.Contains(mailBody, "http://localhost:8080/recover") || !strings.Contains(mailBody, "agentuser") {
		t.Fatalf("mail should contain recover link and username:\n%s", mailBody)
	}
	digitRe := regexp.MustCompile(`>([0-9])</td>`)
	matches := digitRe.FindAllStringSubmatch(mailBody, -1)
	if len(matches) != 6 {
		t.Fatalf("mail should render 6 digit boxes:\n%s", mailBody)
	}
	code := ""
	for _, m := range matches {
		code += m[1]
	}

	// 用登录账号 + 6 位验证码完成兑换。
	performJSON(t, handler, http.MethodPost, "/v1/accounts/recovery", "", map[string]any{
		"username": "agentuser", "code": code, "password": "brand-new-password",
	})
	// 验证码一次性：同一验证码再次兑换应失败。
	performJSONStatus(t, handler, http.MethodPost, "/v1/accounts/recovery", "", map[string]any{
		"username": "agentuser", "code": code, "password": "brand-new-password",
	}, http.StatusUnauthorized)

	// 无邮箱的账号应明确报错。
	var adminAccounts []map[string]any
	decodeData(t, performJSON(t, handler, http.MethodGet, "/admin/recovery/accounts?kind=admin", token, nil), &adminAccounts)
	if len(adminAccounts) == 0 {
		t.Fatal("expected at least the bootstrap admin account")
	}
	adminID, _ := adminAccounts[0]["id"].(string)
	performJSONStatus(t, handler, http.MethodPost, "/admin/recovery/admin/"+adminID+"/email", token, map[string]any{
		"reason": "管理员忘记密码", "minutes": 30,
	}, http.StatusBadRequest)
}
