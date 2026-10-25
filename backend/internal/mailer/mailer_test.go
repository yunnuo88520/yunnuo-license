package mailer

import (
	"html/template"
	"strings"
	"testing"
)

func testConfig() Config {
	return Config{
		Host: "smtp.example.com", Port: 465,
		Username: "noreply@example.com", Password: "secret",
		FromAddress: "noreply@example.com", FromName: "允诺云授权",
		Encryption: EncryptionSSL,
	}
}

func SampleData() Data {
	return Data{
		SiteName: "允诺云授权", SiteURL: "https://example.com",
		SiteLinkText: "访问站点", Preheader: "测试预览文案",
		Title: "测试邮件标题", Greeting: "尊敬的合作伙伴：",
		Intro: "这是一封测试邮件。",
		Rows: []Row{
			{Label: "调整金额", Value: "+100.00 元"},
			{Label: "调整后余额", Value: "1,200.00 元"},
		},
		Button:  Button{Text: "前往登录", URL: "https://example.com/login"},
		Warning: "为保障账号安全，请勿泄露此信息。",
	}
}

func TestValidate(t *testing.T) {
	cfg := testConfig()
	if err := Validate(cfg); err != nil {
		t.Fatalf("合法配置不应报错: %v", err)
	}
	bad := testConfig()
	bad.Host = " "
	if err := Validate(bad); err == nil {
		t.Fatal("空主机名应报错")
	}
	bad = testConfig()
	bad.Port = 0
	if err := Validate(bad); err == nil {
		t.Fatal("非法端口应报错")
	}
	bad = testConfig()
	bad.Encryption = "foo"
	if err := Validate(bad); err == nil {
		t.Fatal("未知加密方式应报错")
	}
	bad = testConfig()
	bad.FromAddress = "not-an-email"
	if err := Validate(bad); err == nil {
		t.Fatal("非法发件人应报错")
	}
	bad = testConfig()
	bad.Username = ""
	bad.Password = "x"
	if err := Validate(bad); err == nil {
		t.Fatal("只有密码没有用户名应报错")
	}
}

func TestBuildMessage(t *testing.T) {
	cfg := testConfig()
	msg, err := BuildMessage(cfg, []string{"Agent@Example.com", "bad..", "agent@example.com"}, "余额变动提醒", "<p>正文</p>")
	if err != nil {
		t.Fatalf("构建邮件失败: %v", err)
	}
	text := string(msg)
	for _, want := range []string{
		"From: =?utf-8?q?", "agent@example.com", "To: agent@example.com",
		"MIME-Version: 1.0", "Content-Type: text/html; charset=utf-8",
		"Content-Transfer-Encoding: base64",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("报文缺少 %q\n%s", want, text)
		}
	}
	if !strings.Contains(text, "?utf-8?q?") {
		t.Errorf("中文主题应使用编码传输\n%s", text)
	}
	if strings.Contains(text, "<p>正文</p>") {
		t.Error("HTML 正文必须经过 base64 编码")
	}
	if n := strings.Count(text, "agent@example.com"); n != 1 {
		t.Errorf("重复收件人应去重，To 头中应只出现 1 次，实际 %d\n%s", n, text)
	}
	if _, err := BuildMessage(cfg, []string{"nope"}, "s", "<p/>"); err == nil {
		t.Error("无合法收件人应报错")
	}
}

func TestRenderTemplates(t *testing.T) {
	html, err := Render(TemplateNotification, SampleData())
	if err != nil {
		t.Fatalf("渲染通知模板失败: %v", err)
	}
	for _, want := range []string{"允诺云授权", "测试邮件标题", "尊敬的合作伙伴", "调整后余额", "前往登录", "系统自动发送"} {
		if !strings.Contains(html, want) {
			t.Errorf("通知模板缺少 %q", want)
		}
	}
	codeHTML, err := Render(TemplateVerifyCode, func() Data {
		d := SampleData()
		d.Rows = nil
		d.BodyHTML = ""
		d.Code = "482913"
		d.ValidMinutes = 30
		d.Button = Button{Text: "打开密码恢复页", URL: "https://example.com/recover"}
		return d
	}())
	if err != nil {
		t.Fatalf("渲染验证码模板失败: %v", err)
	}
	for _, want := range []string{"30 分钟内有效", "打开密码恢复页"} {
		if !strings.Contains(codeHTML, want) {
			t.Errorf("验证码模板缺少 %q", want)
		}
	}
	// 6 位纯数字应渲染为六个独立数字格，每格一位数字。
	if n := strings.Count(codeHTML, "<td align=\"center\" width=\"52\""); n != 6 {
		t.Errorf("6 位验证码应渲染 6 个数字格，实际 %d\n%s", n, codeHTML)
	}
	for _, digit := range []string{"4", "8", "2", "9", "1", "3"} {
		if !strings.Contains(codeHTML, ">"+digit+"</td>") {
			t.Errorf("验证码模板缺少数字格 %q\n%s", digit, codeHTML)
		}
	}
	// 非 6 位数字（如手动长恢复码）回退到单框展示。
	longHTML, err := Render(TemplateVerifyCode, Data{Title: "T", Code: "reset_Ab3dEf7hJk9Lm2Np"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(longHTML, "width=\"52\"") || !strings.Contains(longHTML, "reset_Ab3dEf7hJk9Lm2Np") {
		t.Errorf("长码应回退单框展示\n%s", longHTML)
	}
	if _, err := Render("nope", Data{}); err == nil {
		t.Error("未知模板应报错")
	}
}

func TestManualBodyHTMLEscapes(t *testing.T) {
	out := string(ManualBodyHTML("第一行\n<b>加粗</b>\r\n第二行"))
	if strings.Contains(out, "<b>") {
		t.Errorf("手动正文必须转义: %s", out)
	}
	if !strings.Contains(out, "&lt;b&gt;") || !strings.Contains(out, "<br />") {
		t.Errorf("转义/换行结果不符合预期: %s", out)
	}
}

func TestSiteNameFallback(t *testing.T) {
	html, err := Render(TemplateNotification, Data{Title: "T", BodyHTML: template.HTML("")})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "允诺云授权") {
		t.Error("站名为空时应回退默认值")
	}
}
