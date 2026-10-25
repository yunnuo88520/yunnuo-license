package service

import (
	"time"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
)

// bindingOnlineStates 按产品的离线容忍期计算每个生效绑定的心跳视图：
// online 24 小时内有上报；idle 超过 24 小时但未超容忍期；offline 超过容忍期。
// 从未上报过心跳的绑定按激活时间起算。非生效绑定不参与计算。
func bindingOnlineStates(bindings []domain.Binding, offlineGraceDays int, now time.Time) {
	grace := time.Duration(offlineGraceDays) * 24 * time.Hour
	if grace <= 0 {
		grace = 15 * 24 * time.Hour
	}
	for i := range bindings {
		b := &bindings[i]
		if b.Status != domain.BindingActive {
			continue
		}
		lastSeen := b.ActivatedAt
		if b.LastHeartbeatAt != nil && b.LastHeartbeatAt.After(lastSeen) {
			lastSeen = *b.LastHeartbeatAt
		}
		since := now.Sub(lastSeen)
		b.LastSeenAt = &lastSeen
		b.OfflineDays = int(since.Hours() / 24)
		switch {
		case since <= 24*time.Hour:
			b.OnlineState = "online"
		case since <= grace:
			b.OnlineState = "idle"
		default:
			b.OnlineState = "offline"
		}
	}
}

// maxOfflineGraceDays 限制产品离线缓存宽限上限：每次验证都会重新下发离线
// 令牌，过长的宽限期等于把吊销延迟放大到不可控。
const maxOfflineGraceDays = 365
