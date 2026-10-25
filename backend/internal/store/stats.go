package store

import (
	"context"
	"time"
)

// OverviewStats 仪表盘聚合统计。
type OverviewStats struct {
	Cards struct {
		Total  int64 `json:"total"`
		Unused int64 `json:"unused"`
		Used   int64 `json:"used"`
		Voided int64 `json:"voided"`
	} `json:"cards"`
	Licenses struct {
		Total  int64 `json:"total"`
		Active int64 `json:"active"`
	} `json:"licenses"`
	Agents struct {
		Total  int64 `json:"total"`
		Active int64 `json:"active"`
	} `json:"agents"`
	Products struct {
		Total  int64 `json:"total"`
		Active int64 `json:"active"`
	} `json:"products"`
	Activations []ActivationPoint `json:"activations"`
}

type ActivationPoint struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

// GetOverviewStats 汇总卡密状态分布与近 14 天激活趋势。
func (s *Store) GetOverviewStats(ctx context.Context, seriesStart time.Time) (OverviewStats, error) {
	var stats OverviewStats
	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM cards GROUP BY status`)
	if err != nil {
		return stats, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return stats, err
		}
		stats.Cards.Total += count
		switch status {
		case "unused":
			stats.Cards.Unused = count
		case "activated", "consumed_for_renewal":
			stats.Cards.Used += count
		case "voided":
			stats.Cards.Voided = count
		}
	}
	if err := rows.Err(); err != nil {
		return stats, err
	}

	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(status='active'),0) FROM licenses`).Scan(
		&stats.Licenses.Total, &stats.Licenses.Active); err != nil {
		return stats, err
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(status='active'),0) FROM agents`).Scan(
		&stats.Agents.Total, &stats.Agents.Active); err != nil {
		return stats, err
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(status='active'),0) FROM products`).Scan(
		&stats.Products.Total, &stats.Products.Active); err != nil {
		return stats, err
	}

	byDay := map[string]int64{}
	dayRows, err := s.db.QueryContext(ctx,
		`SELECT SUBSTR(activated_at,1,10) d, COUNT(*) FROM licenses WHERE activated_at IS NOT NULL AND activated_at >= ? GROUP BY d`,
		ts(seriesStart))
	if err != nil {
		return stats, err
	}
	defer dayRows.Close()
	for dayRows.Next() {
		var day string
		var count int64
		if err := dayRows.Scan(&day, &count); err != nil {
			return stats, err
		}
		byDay[day] = count
	}
	if err := dayRows.Err(); err != nil {
		return stats, err
	}

	stats.Activations = []ActivationPoint{}
	for i := 13; i >= 0; i-- {
		day := time.Now().UTC().AddDate(0, 0, -i).Format("2006-01-02")
		stats.Activations = append(stats.Activations, ActivationPoint{Date: day, Count: byDay[day]})
	}
	return stats, nil
}
