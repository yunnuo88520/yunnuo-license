package service

import (
	"bufio"
	"context"
	"encoding/base64"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
)

// fakeSMTPServer 捕获投递内容的最小 SMTP 服务器（无认证、无加密）。
type fakeSMTPServer struct {
	listener net.Listener
	mu       sync.Mutex
	bodies   []string
}

func startFakeSMTPServer(t *testing.T) *fakeSMTPServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &fakeSMTPServer{listener: ln}
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

func (s *fakeSMTPServer) port() int { return s.listener.Addr().(*net.TCPAddr).Port }

// countBody 返回解码后正文包含 marker 的邮件数量。
func (s *fakeSMTPServer) countBody(marker string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, body := range s.bodies {
		if strings.Contains(body, marker) {
			count++
		}
	}
	return count
}

func (s *fakeSMTPServer) serve(conn net.Conn) {
	defer conn.Close()
	write := func(line string) { _, _ = conn.Write([]byte(line + "\r\n")) }
	write("220 test.local ESMTP")
	br := bufio.NewReader(conn)
	inData := false
	inBody := false
	var bodyLines []string
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if inData {
			if trimmed == "." {
				inData = false
				inBody = false
				if decoded, err := base64.StdEncoding.DecodeString(strings.Join(bodyLines, "")); err == nil {
					s.mu.Lock()
					s.bodies = append(s.bodies, string(decoded))
					s.mu.Unlock()
				}
				bodyLines = nil
				write("250 OK")
			} else {
				if trimmed == "" {
					inBody = true
				} else if inBody {
					bodyLines = append(bodyLines, trimmed)
				}
			}
			continue
		}
		upper := strings.ToUpper(trimmed)
		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			write("250-test.local")
			write("250 OK")
		case upper == "DATA":
			inData = true
			write("354 go")
		case upper == "QUIT":
			write("221 bye")
			return
		default:
			write("250 OK")
		}
	}
}

func TestExpiryReminderScanSendsOnceAndRearmsAfterRenewal(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	smtp := startFakeSMTPServer(t)
	product := createTestProduct(t, ctx, svc, domain.ConflictReject)

	if _, err := svc.UpdateEmailSettings(ctx, UpdateEmailSettingsInput{
		Enabled: true, SMTPHost: "127.0.0.1", SMTPPort: smtp.port(),
		FromAddress: "notify@example.com", FromName: "License", Encryption: "none",
	}); err != nil {
		t.Fatalf("email settings: %v", err)
	}

	customer, err := svc.CreateCustomer(ctx, CustomerInput{
		Username: "expiring", Password: "password123", DisplayName: "Expiring User",
		Email: "owner@example.com",
	}, "admin-test")
	if err != nil {
		t.Fatalf("create customer: %v", err)
	}

	// 10 天后到期：命中 30 天阈值，不命中 7 天阈值。
	code := createTestCard(t, ctx, svc, product.ID, 10)
	resp, err := svc.CustomerActivate(ctx, customer.ID, product.Code, ActivateInput{
		CardCode: code, BindMode: domain.BindDevice, BindValue: "expiry-device",
	})
	if err != nil {
		t.Fatalf("activate: %v", err)
	}

	sent, err := svc.RunExpiryReminderScan(ctx)
	if err != nil || sent != 1 {
		t.Fatalf("first scan should send exactly one combined reminder: sent=%d err=%v", sent, err)
	}
	if got := smtp.countBody("使用期限、更新服务"); got != 1 {
		t.Fatalf("expected one combined reminder delivery, got %d", got)
	}
	// 重复扫描不得重复发送。
	sent, err = svc.RunExpiryReminderScan(ctx)
	if err != nil || sent != 0 {
		t.Fatalf("second scan must not resend: sent=%d err=%v", sent, err)
	}
	if got := smtp.countBody("授权到期提醒"); got != 1 {
		t.Fatalf("expected no duplicate delivery, got %d", got)
	}

	// 续期 5 天（新到期时间 now+15d，仍在 30 天窗口内）：新的到期时间重新获得提醒资格。
	renewal := createTestCard(t, ctx, svc, product.ID, 5)
	bindings, err := svc.ListLicenseBindings(ctx, resp.LicenseNo)
	if err != nil || len(bindings) == 0 {
		t.Fatalf("list bindings: %v", err)
	}
	if _, err := svc.CustomerRenew(ctx, customer.ID, resp.LicenseNo, bindings[0].ID, renewal); err != nil {
		t.Fatalf("renew: %v", err)
	}
	sent, err = svc.RunExpiryReminderScan(ctx)
	if err != nil || sent != 1 {
		t.Fatalf("scan after renewal should send for the new expiry: sent=%d err=%v", sent, err)
	}
	if got := smtp.countBody("授权到期提醒"); got != 2 {
		t.Fatalf("expected two reminders in total, got %d", got)
	}
}

func TestExpiryReminderScanSkipsWithoutEmail(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	product := createTestProduct(t, ctx, svc, domain.ConflictReject)
	customer, err := svc.CreateCustomer(ctx, CustomerInput{
		Username: "silent", Password: "password123", DisplayName: "Silent", Email: "quiet@example.com",
	}, "admin-test")
	if err != nil {
		t.Fatalf("create customer: %v", err)
	}
	code := createTestCard(t, ctx, svc, product.ID, 10)
	if _, err := svc.CustomerActivate(ctx, customer.ID, product.Code, ActivateInput{
		CardCode: code, BindMode: domain.BindDevice, BindValue: "silent-device",
	}); err != nil {
		t.Fatalf("activate: %v", err)
	}
	// 邮件未启用：扫描静默返回 0，不报错。
	sent, err := svc.RunExpiryReminderScan(ctx)
	if err != nil || sent != 0 {
		t.Fatalf("scan without email settings should be a no-op: sent=%d err=%v", sent, err)
	}
}
