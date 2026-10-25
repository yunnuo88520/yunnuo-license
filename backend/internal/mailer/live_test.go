package mailer

import (
	"os"
	"testing"
	"time"
)

// TestLiveQQMail 用真实凭据实测 QQ SMTP 发信（仅本地手动运行，凭据不入库）。
func TestLiveQQMail(t *testing.T) {
	host := os.Getenv("LIVE_SMTP_HOST")
	user := os.Getenv("LIVE_SMTP_USER")
	pass := os.Getenv("LIVE_SMTP_PASS")
	if host == "" || user == "" || pass == "" {
		t.Skip("set LIVE_SMTP_HOST/USER/PASS to run")
	}
	cfg := Config{
		Host: host, Port: 465,
		Username: user, Password: pass,
		FromAddress: user, FromName: "允诺云授权",
		Encryption: EncryptionSSL,
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("validate: %v", err)
	}
	html, err := Render(TemplateNotification, Data{
		SiteName: "允诺云授权", Title: "测试邮件",
		Greeting: "您好：", Intro: "这是一封来自允诺云授权系统的测试邮件，收到即说明 SMTP 配置正确。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := Send(cfg, []string{user}, "允诺云授权｜测试邮件", html); err != nil {
		t.Fatalf("send: %v", err)
	}
	_ = time.Now()
}
