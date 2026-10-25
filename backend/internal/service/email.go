package service

import (
	"context"
	"fmt"
	"log"
	netmail "net/mail"
	"strings"
	"time"

	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/mailer"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

const emailSendTimeout = 45 * time.Second

// ---------- SMTP 设置 ----------

type EmailSettingsView struct {
	Enabled      bool      `json:"enabled"`
	SMTPHost     string    `json:"smtp_host"`
	SMTPPort     int       `json:"smtp_port"`
	SMTPUsername string    `json:"smtp_username"`
	HasPassword  bool      `json:"has_password"`
	FromAddress  string    `json:"from_address"`
	FromName     string    `json:"from_name"`
	Encryption   string    `json:"encryption"`
	SiteURL      string    `json:"site_url"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type UpdateEmailSettingsInput struct {
	Enabled      bool   `json:"enabled"`
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUsername string `json:"smtp_username"`
	// Password 为空表示保持已保存的密码不变。
	Password    string `json:"password"`
	FromAddress string `json:"from_address"`
	FromName    string `json:"from_name"`
	Encryption  string `json:"encryption"`
	SiteURL     string `json:"site_url"`
	ActorID     string `json:"-"`
}

func (s *Service) EmailSettings(ctx context.Context) (EmailSettingsView, error) {
	settings, err := s.currentStore().GetEmailSettings(ctx)
	if err != nil {
		return EmailSettingsView{}, err
	}
	return emailSettingsView(settings), nil
}

func (s *Service) UpdateEmailSettings(ctx context.Context, input UpdateEmailSettingsInput) (EmailSettingsView, error) {
	input.SMTPHost = strings.TrimSpace(input.SMTPHost)
	input.SMTPUsername = strings.TrimSpace(input.SMTPUsername)
	input.FromAddress = strings.TrimSpace(input.FromAddress)
	input.FromName = strings.TrimSpace(input.FromName)
	siteURL := normalizeSiteURL(input.SiteURL)
	encryption := input.Encryption
	if encryption == "" {
		encryption = domain.EmailEncryptionSSL
	}
	if input.SMTPPort < 1 || input.SMTPPort > 65535 {
		return EmailSettingsView{}, badRequest("INVALID_REQUEST", "SMTP 端口必须在 1-65535 之间")
	}
	switch encryption {
	case domain.EmailEncryptionNone, domain.EmailEncryptionSTARTTLS, domain.EmailEncryptionSSL:
	default:
		return EmailSettingsView{}, badRequest("INVALID_REQUEST", "不支持的 SMTP 加密方式")
	}
	current, err := s.currentStore().GetEmailSettings(ctx)
	if err != nil {
		return EmailSettingsView{}, err
	}
	// 主流邮箱服务商（QQ/163/Gmail 等）的 SMTP 用户名即完整邮箱地址：留空时
	// 优先保持已保存的用户名，首次保存则自动取发件人地址。
	if input.SMTPUsername == "" {
		if current.SMTPUsername != "" {
			input.SMTPUsername = current.SMTPUsername
		} else if input.Password != "" {
			input.SMTPUsername = input.FromAddress
		}
	}
	cfg := mailer.Config{
		Host: input.SMTPHost, Port: input.SMTPPort, Username: input.SMTPUsername,
		Password: input.Password, FromAddress: input.FromAddress,
		FromName: input.FromName, Encryption: encryption,
	}
	if input.Enabled {
		if err := mailer.Validate(cfg); err != nil {
			return EmailSettingsView{}, badRequest("INVALID_REQUEST", err.Error())
		}
		// 无用户名的中继不需要认证；有用户名时必须已有或新填密码。
		if input.Password == "" && input.SMTPUsername != "" && current.SMTPPasswordEnc == "" {
			return EmailSettingsView{}, badRequest("INVALID_REQUEST", "SMTP 密码不能为空")
		}
	}
	settings := domain.EmailSettings{
		Enabled: boolToInt(input.Enabled), SMTPHost: input.SMTPHost, SMTPPort: input.SMTPPort,
		SMTPUsername: input.SMTPUsername, FromAddress: input.FromAddress,
		FromName: input.FromName, Encryption: encryption, SiteURL: siteURL,
		UpdatedAt: s.now(),
	}
	if input.Password != "" {
		enc, err := yncrypto.EncryptString(s.dataKey, input.Password)
		if err != nil {
			return EmailSettingsView{}, err
		}
		settings.SMTPPasswordEnc = enc
	} else {
		settings.SMTPPasswordEnc = current.SMTPPasswordEnc
	}
	if err := s.currentStore().UpdateEmailSettings(ctx, settings); err != nil {
		return EmailSettingsView{}, err
	}
	_ = s.audit(ctx, "admin", input.ActorID, "", "", "", "email_settings.update", "success", "")
	return emailSettingsView(settings), nil
}

func emailSettingsView(s domain.EmailSettings) EmailSettingsView {
	return EmailSettingsView{
		Enabled: s.Enabled == 1, SMTPHost: s.SMTPHost, SMTPPort: s.SMTPPort,
		SMTPUsername: s.SMTPUsername, HasPassword: s.SMTPPasswordEnc != "",
		FromAddress: s.FromAddress, FromName: s.FromName,
		Encryption: s.Encryption, SiteURL: s.SiteURL, UpdatedAt: s.UpdatedAt,
	}
}

func normalizeSiteURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
		value = "https://" + value
	}
	return strings.TrimRight(value, "/")
}

// normalizeEmail 归一并校验邮箱：小写、去空白、必须可解析；空值合法（表示未绑定）。
func normalizeEmail(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", nil
	}
	addr, err := netmail.ParseAddress(value)
	if err != nil || addr.Address != value || len(value) > 254 {
		return "", badRequest("EMAIL_INVALID", "邮箱地址格式不正确")
	}
	return value, nil
}

// ensureEmailAvailable 校验邮箱在指定账号表中未被占用。
func (s *Service) ensureEmailAvailable(ctx context.Context, table, email string) error {
	if email == "" {
		return nil
	}
	taken, err := s.currentStore().EmailTaken(ctx, table, email)
	if err != nil {
		return err
	}
	if taken {
		return badRequest("EMAIL_TAKEN", "该邮箱已被其他账号使用")
	}
	return nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// ---------- 发送 ----------

type emailRuntime struct {
	cfg      mailer.Config
	settings domain.EmailSettings
}

// emailRuntime 加载邮件设置并解密 SMTP 密码。
func (s *Service) emailRuntime(ctx context.Context) (*emailRuntime, error) {
	settings, err := s.currentStore().GetEmailSettings(ctx)
	if err != nil {
		return nil, err
	}
	r := &emailRuntime{settings: settings}
	r.cfg = mailer.Config{
		Host: settings.SMTPHost, Port: settings.SMTPPort, Username: settings.SMTPUsername,
		FromAddress: settings.FromAddress, FromName: settings.FromName, Encryption: settings.Encryption,
	}
	if settings.SMTPPasswordEnc != "" {
		password, err := yncrypto.DecryptString(s.dataKey, settings.SMTPPasswordEnc)
		if err != nil {
			return nil, fmt.Errorf("解密 SMTP 密码失败: %w", err)
		}
		r.cfg.Password = password
	}
	return r, nil
}

// usable 报告邮件功能已启用且配置完整，自动通知只在此时发送。
func (r *emailRuntime) usable() bool {
	return r.settings.Enabled == 1 && r.cfg.Ready()
}

func (r *emailRuntime) baseData(ctx context.Context, s *Service) mailer.Data {
	data := mailer.Data{SiteName: "允诺云授权", SiteURL: r.settings.SiteURL}
	if settings, err := s.currentStore().GetSiteSettings(ctx); err == nil {
		data.SiteName = settings.SiteName
		data.LogoDataURL = settings.LogoDataURL
	}
	if data.SiteURL != "" {
		data.SiteLinkText = "访问 " + data.SiteName
	}
	return data
}

func (s *Service) deliverEmail(ctx context.Context, r *emailRuntime, to []string, subject, kind, agentID, html string) error {
	sendErr := mailer.Send(r.cfg, to, subject, html)
	s.recordEmailLog(ctx, to, subject, kind, agentID, sendErr)
	return sendErr
}

func (s *Service) recordEmailLog(ctx context.Context, to []string, subject, kind, agentID string, sendErr error) {
	status, errMsg := "sent", ""
	if sendErr != nil {
		status, errMsg = "failed", sendErr.Error()
	}
	entry := domain.EmailLog{
		ID: yncrypto.NewID("mail"), ToAddress: truncateRunes(strings.Join(to, ", "), 250),
		Subject: truncateRunes(subject, 250), Kind: kind, AgentID: agentID,
		Status: status, Error: truncateRunes(errMsg, 450), CreatedAt: s.now(),
	}
	if err := s.currentStore().InsertEmailLog(ctx, entry); err != nil {
		log.Printf("email log insert failed kind=%s status=%s: %v", kind, status, err)
	}
}

func (s *Service) EmailLogs(ctx context.Context, agentID string, limit int) ([]domain.EmailLog, error) {
	return s.currentStore().ListEmailLogs(ctx, agentID, limit)
}

// ---------- 测试与手动发送 ----------

func (s *Service) SendTestEmail(ctx context.Context, actorID, to string) error {
	to = strings.TrimSpace(to)
	r, err := s.emailRuntime(ctx)
	if err != nil {
		return err
	}
	if !r.usable() {
		return badRequest("EMAIL_NOT_ENABLED", "请先保存配置并启用邮件功能")
	}
	if err := mailer.Validate(r.cfg); err != nil {
		return badRequest("EMAIL_NOT_CONFIGURED", "请先完整保存 SMTP 配置: "+err.Error())
	}
	data := r.baseData(ctx, s)
	data.Title = "测试邮件"
	data.Preheader = "这是一封来自 " + data.SiteName + " 的测试邮件"
	data.Greeting = "您好："
	data.Intro = "这是一封测试邮件。收到即说明 SMTP 配置正确，代理通知邮件可以正常送达。"
	data.Button = mailer.Button{}
	data.Warning = ""
	html, err := mailer.Render(mailer.TemplateNotification, data)
	if err != nil {
		return err
	}
	if err := s.deliverEmail(ctx, r, []string{to}, data.SiteName+"｜测试邮件", domain.EmailKindTest, "", html); err != nil {
		return badRequest("EMAIL_SEND_FAILED", err.Error())
	}
	_ = s.audit(ctx, "admin", actorID, "", "", "", "email.test", "success", to)
	return nil
}

type SendAgentEmailInput struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
	ActorID string `json:"-"`
}

// SendAgentEmail 管理员手动给代理发邮件，返回实际收件人数。
func (s *Service) SendAgentEmail(ctx context.Context, agentID string, input SendAgentEmailInput) (int, error) {
	agentID = strings.TrimSpace(agentID)
	input.Subject = strings.TrimSpace(input.Subject)
	input.Body = strings.TrimSpace(input.Body)
	if agentID == "" || input.Subject == "" || input.Body == "" || len([]rune(input.Subject)) > 200 || len([]rune(input.Body)) > 5000 {
		return 0, ErrInvalidRequest
	}
	agent, err := s.currentStore().GetAgent(ctx, agentID)
	if err != nil {
		if store.IsNotFound(err) {
			return 0, ErrAgentNotFound
		}
		return 0, err
	}
	to, err := s.currentStore().AgentEmailRecipients(ctx, agentID)
	if err != nil {
		return 0, err
	}
	if len(to) == 0 {
		return 0, badRequest("AGENT_EMAIL_MISSING", "该代理没有可用的邮箱地址，请先在代理资料中补充邮箱")
	}
	r, err := s.emailRuntime(ctx)
	if err != nil {
		return 0, err
	}
	if !r.usable() {
		return 0, badRequest("EMAIL_NOT_ENABLED", "请先保存配置并启用邮件功能")
	}
	if err := mailer.Validate(r.cfg); err != nil {
		return 0, badRequest("EMAIL_NOT_CONFIGURED", "请先完整保存 SMTP 配置: "+err.Error())
	}
	data := r.baseData(ctx, s)
	data.Title = input.Subject
	data.Preheader = input.Subject
	data.Greeting = "尊敬的 " + agent.Name + "："
	data.BodyHTML = mailer.ManualBodyHTML(input.Body)
	data.Button = mailer.Button{}
	data.Warning = ""
	html, err := mailer.Render(mailer.TemplateNotification, data)
	if err != nil {
		return 0, err
	}
	if err := s.deliverEmail(ctx, r, to, input.Subject, domain.EmailKindManual, agentID, html); err != nil {
		return 0, badRequest("EMAIL_SEND_FAILED", err.Error())
	}
	_ = s.audit(ctx, "admin", input.ActorID, "", "", "", "email.send", "success", agent.Name)
	return len(to), nil
}

// ---------- 验证码 ----------

type verifyCodeEmail struct {
	Code         string
	Purpose      string
	Greeting     string
	Intro        string
	ValidMinutes int
	ActionText   string
	ActionURL    string
	Warning      string
}

// sendVerificationEmail 渲染验证码模板并同步发送，供恢复码及未来验证码场景复用。
func (s *Service) sendVerificationEmail(ctx context.Context, r *emailRuntime, to []string, kind, agentID string, v verifyCodeEmail) error {
	data := r.baseData(ctx, s)
	data.Title = v.Purpose
	data.Preheader = v.Purpose + "：" + v.Code
	data.Greeting = v.Greeting
	data.Intro = v.Intro
	if data.Intro == "" {
		data.Intro = "请使用下方验证码完成操作："
	}
	data.Code = v.Code
	data.ValidMinutes = v.ValidMinutes
	if v.ActionText != "" && v.ActionURL != "" {
		data.Button = mailer.Button{Text: v.ActionText, URL: v.ActionURL}
	}
	data.Warning = v.Warning
	if data.Warning == "" {
		data.Warning = "为保障账号安全，请勿将验证码透露给任何人；若非本人操作，请忽略此邮件。"
	}
	html, err := mailer.Render(mailer.TemplateVerifyCode, data)
	if err != nil {
		return err
	}
	subject := data.SiteName + "｜" + v.Purpose
	return s.deliverEmail(ctx, r, to, subject, kind, agentID, html)
}

// SendVerificationCode 发送一封通用验证码邮件（同步），供后续邮箱绑定等场景复用。
func (s *Service) SendVerificationCode(ctx context.Context, to, code, purpose string, validMinutes int, actionText, actionURL string) error {
	r, err := s.emailRuntime(ctx)
	if err != nil {
		return err
	}
	if !r.usable() {
		return badRequest("EMAIL_NOT_ENABLED", "邮件功能未启用或 SMTP 配置不完整")
	}
	if err := mailer.Validate(r.cfg); err != nil {
		return badRequest("EMAIL_NOT_CONFIGURED", "SMTP 配置不完整: "+err.Error())
	}
	return s.sendVerificationEmail(ctx, r, []string{strings.TrimSpace(to)}, "verify_code", "", verifyCodeEmail{
		Code: code, Purpose: purpose, Greeting: "您好：",
		ValidMinutes: validMinutes, ActionText: actionText, ActionURL: actionURL,
	})
}

// ---------- 自动通知 ----------

// notifyAgentEmail 异步发送代理通知：不阻塞业务事务，未启用或无收件人时静默跳过。
func (s *Service) notifyAgentEmail(kind, agentID, subject string, build func(base mailer.Data) mailer.Data) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("agent email notify panic kind=%s agent=%s: %v", kind, agentID, r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), emailSendTimeout)
		defer cancel()
		if s.currentStore() == nil {
			return
		}
		r, err := s.emailRuntime(ctx)
		if err != nil {
			log.Printf("agent email notify skipped kind=%s agent=%s: %v", kind, agentID, err)
			return
		}
		if !r.usable() {
			return
		}
		to, err := s.currentStore().AgentEmailRecipients(ctx, agentID)
		if err != nil {
			log.Printf("agent email recipients failed kind=%s agent=%s: %v", kind, agentID, err)
			return
		}
		if len(to) == 0 {
			return
		}
		html, err := mailer.Render(mailer.TemplateNotification, build(r.baseData(ctx, s)))
		if err != nil {
			log.Printf("agent email render failed kind=%s agent=%s: %v", kind, agentID, err)
			return
		}
		if err := s.deliverEmail(ctx, r, to, subject, kind, agentID, html); err != nil {
			log.Printf("agent email notify failed kind=%s agent=%s: %v", kind, agentID, err)
		}
	}()
}

// notifyAdminRiskBlock 异步向全部启用邮箱的管理员发送风控自动封禁通知：
// 未启用邮件或无收件人时静默跳过；仅在封禁创建时触发一次，不会重复发送。
func (s *Service) notifyAdminRiskBlock(ip, reason string, expiresAt time.Time) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("admin risk email notify panic: %v", r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), emailSendTimeout)
		defer cancel()
		if s.currentStore() == nil {
			return
		}
		r, err := s.emailRuntime(ctx)
		if err != nil || !r.usable() {
			return
		}
		to, err := s.currentStore().AdminEmailRecipients(ctx)
		if err != nil || len(to) == 0 {
			return
		}
		base := r.baseData(ctx, s)
		base.Title = "风控自动封禁通知"
		base.Preheader = base.SiteName + "风控自动封禁通知"
		base.Greeting = "管理员您好："
		base.Intro = "系统已自动封禁一个存在持续激活试探行为的 IP，请知悉并视情况处理："
		base.Rows = []mailer.Row{
			{Label: "封禁对象", Value: maskRiskValue(domain.RiskBlockIP, ip)},
			{Label: "封禁原因", Value: reason},
			{Label: "自动解除时间", Value: expiresAt.UTC().Format("2006-01-02 15:04 UTC")},
			{Label: "处理方式", Value: "到期自动解除；确认为误伤可在管理后台「安全」中手动解除"},
		}
		base.Warning = "若为共享出口 IP 误伤，请尽快手动解除并评估调整防护策略。"
		html, err := mailer.Render(mailer.TemplateNotification, base)
		if err != nil {
			log.Printf("admin risk email render failed: %v", err)
			return
		}
		if err := s.deliverEmail(ctx, r, to, base.SiteName+"｜风控自动封禁通知", domain.EmailKindRisk, "", html); err != nil {
			log.Printf("admin risk email notify failed: %v", err)
		}
	}()
}

func yuan(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s%.2f 元", sign, float64(cents)/100)
}

func signedYuan(cents int64) string {
	if cents > 0 {
		return "+" + yuan(cents)
	}
	return yuan(cents)
}

func (s *Service) notifyAgentBalance(agentID string, ledger domain.MoneyLedger) {
	s.notifyAgentEmail(domain.EmailKindBalance, agentID, "余额变动通知", func(base mailer.Data) mailer.Data {
		direction := "充值"
		if ledger.Amount < 0 {
			direction = "扣减"
		}
		agentName := agentID
		if agent, err := s.currentStore().GetAgent(context.Background(), agentID); err == nil {
			agentName = agent.Name
		}
		base.Title = "账户余额变动通知"
		base.Preheader = base.SiteName + "账户余额变动通知"
		base.Greeting = "尊敬的 " + agentName + "："
		base.Intro = "您的账户余额发生以下变动，请注意查收："
		base.Rows = []mailer.Row{
			{Label: "变动类型", Value: direction},
			{Label: "变动金额", Value: signedYuan(ledger.Amount)},
			{Label: "调整后余额", Value: yuan(ledger.BalanceAfter)},
			{Label: "备注", Value: ledger.Remark},
			{Label: "操作时间", Value: ledger.CreatedAt.Format("2006-01-02 15:04:05 UTC")},
		}
		base.Warning = "如对变动有疑问，请联系平台管理员核实。"
		return base
	})
}

func (s *Service) notifyAgentQuota(agentID string, ledger domain.AgentQuotaLedger, productName string) {
	s.notifyAgentEmail(domain.EmailKindQuota, agentID, "额度发放通知", func(base mailer.Data) mailer.Data {
		agentName := agentID
		if agent, err := s.currentStore().GetAgent(context.Background(), agentID); err == nil {
			agentName = agent.Name
		}
		term := "永久"
		if !ledger.IsPermanent {
			term = fmt.Sprintf("%d 天", ledger.DurationDays)
		}
		base.Title = "卡密额度发放通知"
		base.Preheader = base.SiteName + "卡密额度发放通知"
		base.Greeting = "尊敬的 " + agentName + "："
		base.Intro = "管理员已为您发放卡密额度："
		base.Rows = []mailer.Row{
			{Label: "产品", Value: productName},
			{Label: "有效期", Value: term},
			{Label: "发放数量", Value: fmt.Sprintf("%d 张", ledger.ChangeQuantity)},
			{Label: "发放后余额", Value: fmt.Sprintf("%d 张", ledger.BalanceAfter)},
			{Label: "备注", Value: ledger.Remark},
			{Label: "操作时间", Value: ledger.CreatedAt.Format("2006-01-02 15:04:05 UTC")},
		}
		return base
	})
}

func (s *Service) notifyAgentAccountCreated(agentID string, user domain.AgentUser, agentName, loginURL string) {
	s.notifyAgentEmail(domain.EmailKindAccount, agentID, "代理账号开通通知", func(base mailer.Data) mailer.Data {
		base.Title = "代理账号开通通知"
		base.Preheader = base.SiteName + "代理账号开通通知"
		base.Greeting = "尊敬的 " + agentName + "："
		base.Intro = "平台已为您开通代理登录账号，使用以下账号登录代理控制台："
		base.Rows = []mailer.Row{
			{Label: "登录账号", Value: user.Username},
			{Label: "账号角色", Value: agentRoleLabel(user.Role)},
		}
		if loginURL != "" {
			base.Button = mailer.Button{Text: "前往登录", URL: loginURL}
		} else {
			base.Rows = append(base.Rows, mailer.Row{Label: "登录地址", Value: "请联系平台管理员获取"})
		}
		base.Warning = "登录密码由平台管理员另行提供，首次登录后请及时修改。"
		return base
	})
}

func agentRoleLabel(role string) string {
	switch role {
	case domain.AgentRoleOwner:
		return "负责人"
	case domain.AgentRoleManager:
		return "管理员"
	case domain.AgentRoleStaff:
		return "操作员"
	case domain.AgentRoleReadonly:
		return "只读"
	default:
		return role
	}
}

func agentStatusLabel(status string) string {
	switch status {
	case domain.AgentActive:
		return "已启用（正常合作中）"
	case domain.AgentSuspended:
		return "已暂停（暂时无法发卡）"
	case domain.AgentDisabled:
		return "已停用（账号不可登录）"
	default:
		return status
	}
}

type RecoveryEmailResult struct {
	SentTo    string    `json:"sent_to"`
	ExpiresAt time.Time `json:"expires_at"`
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "…"
}

func (s *Service) notifyAgentStatus(agent domain.Agent) {
	s.notifyAgentEmail(domain.EmailKindStatus, agent.ID, "代理状态变更通知", func(base mailer.Data) mailer.Data {
		base.Title = "代理状态变更通知"
		base.Preheader = base.SiteName + "代理状态变更通知"
		base.Greeting = "尊敬的 " + agent.Name + "："
		base.Intro = "您的代理状态发生以下变更："
		base.Rows = []mailer.Row{
			{Label: "代理名称", Value: agent.Name},
			{Label: "当前状态", Value: agentStatusLabel(agent.Status)},
			{Label: "变更时间", Value: agent.UpdatedAt.Format("2006-01-02 15:04:05 UTC")},
		}
		return base
	})
}
