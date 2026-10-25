package mailer

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"strings"
)

//go:embed templates/*.html
var templateFS embed.FS

const (
	// TemplateNotification 业务通知与手动邮件共用。
	TemplateNotification = "notification"
	// TemplateVerifyCode 验证码/一次性恢复码类邮件。
	TemplateVerifyCode = "verify_code"
)

type Row struct {
	Label string
	Value string
}

type Button struct {
	Text string
	URL  string
}

type Data struct {
	SiteName     string
	SiteURL      string
	SiteLinkText string
	LogoDataURL  string
	Preheader    string
	Title        string
	Greeting     string
	Intro        string
	// BodyHTML 由调用方转义后的自由正文（手动邮件使用）。
	BodyHTML template.HTML
	Rows     []Row
	Button   Button
	// VerifyCode 模板使用。Code 为 6 位纯数字时自动按 6 个数字格渲染。
	Code         string
	Digits       []string
	ValidMinutes int
	Warning      string
}

// Render 渲染指定模板为一封完整 HTML 邮件正文。
func Render(name string, data Data) (string, error) {
	switch name {
	case TemplateNotification, TemplateVerifyCode:
	default:
		return "", fmt.Errorf("mailer: 未知模板 %q", name)
	}
	if strings.TrimSpace(data.SiteName) == "" {
		data.SiteName = "允诺云授权"
	}
	data.Digits = splitDigits(data.Code)
	tmpl, err := template.New("layout").ParseFS(templateFS,
		"templates/layout.html", "templates/"+name+".html")
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "layout", data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// splitDigits 把 6 位纯数字验证码拆为单字符列表，供六连格渲染；其余返回 nil。
func splitDigits(code string) []string {
	if len(code) != 6 {
		return nil
	}
	digits := make([]string, 0, 6)
	for _, r := range code {
		if r < '0' || r > '9' {
			return nil
		}
		digits = append(digits, string(r))
	}
	return digits
}
