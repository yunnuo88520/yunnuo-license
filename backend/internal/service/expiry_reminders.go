package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/mailer"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

// 到期提醒：对绑定用户账号且留有邮箱的有效授权，在使用期限/更新期限
// 到期前 30 天与 7 天各提醒一次。续期后到期时间变化，会按新的到期时间
// 重新获得提醒资格；同一 (license, kind, 到期时间) 只发送一封。
const (
	expiryReminderFirstDays  = 30
	expiryReminderFinalDays  = 7
	expiryReminderScanTicker = time.Hour
)

func expiryReminderKind(subject string, days int) string {
	return fmt.Sprintf("%s_%dd", subject, days)
}

// reminderRef 是一次待发送提醒中的一个到期主体（使用期限或更新服务）。
type reminderRef struct {
	label     string
	kind      string
	threshold int
	at        time.Time
}

// RunExpiryReminderScan 扫描一次临期授权并发送提醒邮件。
// 返回本次新发送的提醒数量；邮件功能未启用时静默返回 0。
func (s *Service) RunExpiryReminderScan(ctx context.Context) (int, error) {
	r, err := s.emailRuntime(ctx)
	if err != nil {
		return 0, err
	}
	if !r.usable() {
		return 0, nil
	}
	now := s.now()
	horizon := now.Add(expiryReminderFirstDays * 24 * time.Hour)
	items, err := s.currentStore().ListExpiringLicenses(ctx, now, horizon)
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, item := range items {
		// 同一授权的使用/更新期限常同时到期：相同阈值与到期时间的提醒合并为一封邮件。
		groups := map[string][]reminderRef{}
		for _, subject := range []struct {
			label string
			kind  string
			at    *time.Time
		}{
			{label: "使用期限", kind: "usage", at: item.UsageExpiredAt},
			{label: "更新服务", kind: "updates", at: item.UpdatesExpired},
		} {
			if subject.at == nil || subject.at.Before(now) {
				continue
			}
			days := int(subject.at.Sub(now).Hours()/24) + 1
			var threshold int
			switch {
			case days <= expiryReminderFinalDays:
				threshold = expiryReminderFinalDays
			case days <= expiryReminderFirstDays:
				threshold = expiryReminderFirstDays
			default:
				continue
			}
			key := fmt.Sprintf("%d|%s", threshold, subject.at.UTC().Format(time.RFC3339))
			groups[key] = append(groups[key], reminderRef{
				label: subject.label, kind: subject.kind, threshold: threshold, at: *subject.at,
			})
		}
		for _, refs := range groups {
			queued, err := s.sendExpiryReminderEmail(ctx, r, item, refs)
			if err != nil {
				log.Printf("expiry reminder enqueue failed license=%s: %v", item.LicenseNo, err)
				return sent, err
			}
			if !queued {
				continue
			}
			sent++
		}
	}
	return sent, nil
}

func (s *Service) sendExpiryReminderEmail(ctx context.Context, r *emailRuntime, item store.ExpiringLicense, refs []reminderRef) (bool, error) {
	labels := make([]string, 0, len(refs))
	for _, ref := range refs {
		labels = append(labels, ref.label)
	}
	labelText := strings.Join(labels, "、")
	expiry := refs[0].at
	days := int(expiry.Sub(s.now()).Hours()/24) + 1
	data := r.baseData(ctx, s)
	data.Title = "授权到期提醒"
	data.Preheader = fmt.Sprintf("「%s」的%s将在 %d 天后到期", item.ProductName, labelText, days)
	data.Greeting = "您好："
	data.Intro = fmt.Sprintf("您名下产品「%s」的授权即将到期，请及时安排续费：", item.ProductName)
	data.Rows = []mailer.Row{
		{Label: "授权编号", Value: item.LicenseNo},
		{Label: "到期类型", Value: labelText},
		{Label: "到期时间", Value: expiry.UTC().Format("2006-01-02 15:04 UTC")},
		{Label: "剩余天数", Value: fmt.Sprintf("约 %d 天", days)},
	}
	if len(refs) == 1 && refs[0].kind == "updates" {
		data.Warning = "更新服务到期后无法再检查更新和下载新版本，已安装版本的使用不受影响。"
	} else {
		data.Warning = "使用期限到期后应用将无法继续使用（产品配置宽限期的除外）；更新服务到期仅影响获取新版本，不影响既有版本使用。"
	}
	if data.SiteURL != "" {
		data.Button = mailer.Button{Text: "登录用户中心续费", URL: data.SiteURL + "/user-console/"}
	}
	html, err := mailer.Render(mailer.TemplateNotification, data)
	if err != nil {
		return false, err
	}
	subject := fmt.Sprintf("%s｜授权到期提醒（%s 剩约 %d 天）", data.SiteName, labelText, days)
	dedup := fmt.Sprintf("expiry|%s|%d|%s", item.LicenseID, refs[0].threshold, refs[0].at.UTC().Format(time.RFC3339))
	return s.queueEmail(ctx, []string{item.Email}, subject, domain.EmailKindExpiry, "", html, dedup)
}

// StartExpiryReminderScheduler 启动每小时一次的到期提醒扫描，随 Service 关闭停止。
func (s *Service) StartExpiryReminderScheduler() {
	s.expiryOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		s.stopExpiryScan = cancel
		s.expiryDone = make(chan struct{})
		go func() {
			defer close(s.expiryDone)
			timer := time.NewTimer(time.Minute)
			defer timer.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-timer.C:
				}
				scanCtx, scanCancel := context.WithTimeout(ctx, 10*time.Minute)
				if _, err := s.RunExpiryReminderScan(scanCtx); err != nil {
					log.Printf("expiry reminder scan failed: %v", err)
				}
				scanCancel()
				timer.Reset(expiryReminderScanTicker)
			}
		}()
	})
}

func (s *Service) stopExpiryReminderScheduler() {
	if s.stopExpiryScan != nil {
		s.stopExpiryScan()
		<-s.expiryDone
	}
}
