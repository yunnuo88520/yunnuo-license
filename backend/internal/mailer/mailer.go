// Package mailer 提供基于标准库的 SMTP 邮件发送与 HTML 邮件模板渲染。
package mailer

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

const (
	EncryptionNone     = "none"
	EncryptionSTARTTLS = "starttls"
	EncryptionSSL      = "ssl"
)

type Config struct {
	Host        string
	Port        int
	Username    string
	Password    string
	FromAddress string
	FromName    string
	Encryption  string
}

// Ready 报告配置是否已填到足以尝试发送的程度。
func (c Config) Ready() bool {
	return strings.TrimSpace(c.Host) != "" && c.Port > 0 && strings.TrimSpace(c.FromAddress) != ""
}

func Validate(cfg Config) error {
	if strings.TrimSpace(cfg.Host) == "" {
		return errors.New("SMTP 服务器地址不能为空")
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return errors.New("SMTP 端口必须在 1-65535 之间")
	}
	switch cfg.Encryption {
	case EncryptionNone, EncryptionSTARTTLS, EncryptionSSL:
	default:
		return fmt.Errorf("不支持的加密方式 %q", cfg.Encryption)
	}
	if cfg.Username == "" && cfg.Password != "" {
		return errors.New("填写了密码时必须同时填写用户名")
	}
	addr, err := mail.ParseAddress(strings.TrimSpace(cfg.FromAddress))
	if err != nil {
		return errors.New("发件人地址格式不正确")
	}
	if addr.Address != strings.TrimSpace(cfg.FromAddress) {
		return errors.New("发件人地址格式不正确")
	}
	return nil
}

// Send 按 cfg 建立连接并发送一封 HTML 邮件。to 中重复或非法地址会被忽略，
// 全部非法时返回错误。
func Send(cfg Config, to []string, subject, html string) error {
	recipients, err := normalizeRecipients(to)
	if err != nil {
		return err
	}
	msg, err := BuildMessage(cfg, recipients, subject, html)
	if err != nil {
		return err
	}
	addr := net.JoinHostPort(strings.TrimSpace(cfg.Host), fmt.Sprint(cfg.Port))
	host := strings.TrimSpace(cfg.Host)

	var client *smtp.Client
	switch cfg.Encryption {
	case EncryptionSSL:
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host})
		if err != nil {
			return fmt.Errorf("连接 SMTP 服务器失败: %w", err)
		}
		client, err = smtp.NewClient(conn, host)
		if err != nil {
			conn.Close()
			return fmt.Errorf("初始化 SMTP 会话失败: %w", err)
		}
	default:
		conn, err := net.DialTimeout("tcp", addr, 15*time.Second)
		if err != nil {
			return fmt.Errorf("连接 SMTP 服务器失败: %w", err)
		}
		client, err = smtp.NewClient(conn, host)
		if err != nil {
			conn.Close()
			return fmt.Errorf("初始化 SMTP 会话失败: %w", err)
		}
		if cfg.Encryption == EncryptionSTARTTLS {
			if ok, _ := client.Extension("STARTTLS"); !ok {
				client.Close()
				return errors.New("服务器不支持 STARTTLS，请改用 SSL 或无加密方式")
			}
			if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
				client.Close()
				return fmt.Errorf("STARTTLS 升级失败: %w", err)
			}
		}
	}
	defer client.Close()

	if cfg.Username != "" {
		auth := smtp.PlainAuth("", cfg.Username, cfg.Password, host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP 认证失败: %w", err)
		}
	}
	from, _ := mail.ParseAddress(strings.TrimSpace(cfg.FromAddress))
	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("设置发件人失败: %w", err)
	}
	for _, rcpt := range recipients {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("收件人 %s 被拒绝: %w", rcpt, err)
		}
	}
	data, err := client.Data()
	if err != nil {
		return fmt.Errorf("打开邮件数据通道失败: %w", err)
	}
	if _, err := data.Write(msg); err != nil {
		data.Close()
		return fmt.Errorf("写入邮件内容失败: %w", err)
	}
	if err := data.Close(); err != nil {
		return fmt.Errorf("发送邮件失败: %w", err)
	}
	return client.Quit()
}

func normalizeRecipients(to []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, raw := range to {
		addr, err := mail.ParseAddress(strings.TrimSpace(raw))
		if err != nil || seen[strings.ToLower(addr.Address)] {
			continue
		}
		seen[strings.ToLower(addr.Address)] = true
		out = append(out, strings.ToLower(addr.Address))
	}
	if len(out) == 0 {
		return nil, errors.New("没有合法的收件人邮箱地址")
	}
	return out, nil
}

// BuildMessage 组装符合 RFC 5322 的 MIME 邮件报文（HTML，base64 传输编码）。
func BuildMessage(cfg Config, to []string, subject, html string) ([]byte, error) {
	fromAddr := strings.TrimSpace(cfg.FromAddress)
	from, err := mail.ParseAddress(fromAddr)
	if err != nil {
		return nil, errors.New("发件人地址格式不正确")
	}
	recipients, err := normalizeRecipients(to)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString("From: " + formatAddress(cfg.FromName, from.Address) + "\r\n")
	b.WriteString("To: " + strings.Join(recipients, ", ") + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n")
	b.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("Message-ID: <" + messageID(from.Address) + ">\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n")
	b.WriteString("\r\n")
	body := base64.StdEncoding.EncodeToString([]byte(html))
	for len(body) > 76 {
		b.WriteString(body[:76])
		b.WriteString("\r\n")
		body = body[76:]
	}
	b.WriteString(body)
	b.WriteString("\r\n")
	return []byte(b.String()), nil
}

func formatAddress(name, address string) string {
	if strings.TrimSpace(name) == "" {
		return "<" + address + ">"
	}
	return mime.QEncoding.Encode("utf-8", strings.TrimSpace(name)) + " <" + address + ">"
}

func messageID(fromAddress string) string {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Sprintf("%d@yunnuo-license", time.Now().UnixNano())
	}
	domain := "yunnuo-license"
	if at := strings.LastIndex(fromAddress, "@"); at >= 0 && at+1 < len(fromAddress) {
		domain = fromAddress[at+1:]
	}
	return hex.EncodeToString(raw) + "@" + domain
}
