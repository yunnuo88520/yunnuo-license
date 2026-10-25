package service

import (
	"context"
	"time"
)

// OverviewStats 返回仪表盘聚合统计（卡密状态分布 + 近 14 天激活趋势）。
func (s *Service) OverviewStats(ctx context.Context) (interface{}, error) {
	return s.currentStore().GetOverviewStats(ctx, s.now().AddDate(0, 0, -13).Truncate(24*time.Hour))
}
