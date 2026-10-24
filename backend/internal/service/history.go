package service

import (
	"context"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"strings"
	"time"
)

func normalizeHistory(kind, scope, subject string, f domain.HistoryFilter) (domain.HistoryFilter, error) {
	if scope != "admin" && scope != "agent" && scope != "customer" || scope != "admin" && subject == "" {
		return f, ErrPermissionDenied
	}
	if kind != "money" && kind != "cases" && kind != "appeals" {
		return f, ErrInvalidRequest
	}
	if scope == "customer" && kind == "money" || scope == "agent" && kind != "money" {
		return f, ErrPermissionDenied
	}
	f.Query = strings.TrimSpace(f.Query)
	if len(f.Query) > 100 || len(f.ProductID) > 64 || len(f.AgentID) > 64 || f.Page > 10000000 {
		return f, ErrInvalidRequest
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 20
	}
	if f.PageSize > 100 {
		f.PageSize = 100
	}
	for _, value := range []string{f.From, f.To} {
		if value != "" {
			if _, err := time.Parse(time.RFC3339, value); err != nil {
				return f, ErrInvalidRequest
			}
		}
	}
	if f.From != "" && f.To != "" {
		start, _ := time.Parse(time.RFC3339, f.From)
		end, _ := time.Parse(time.RFC3339, f.To)
		if !start.Before(end) {
			return f, ErrInvalidRequest
		}
	}
	for _, field := range []*string{&f.From, &f.To} {
		if *field != "" {
			parsed, _ := time.Parse(time.RFC3339, *field)
			*field = parsed.UTC().Format("2006-01-02T15:04:05.000000000Z")
		}
	}
	if f.Status != "" && (kind == "money" || kind == "cases" && f.Status != "open" && f.Status != "confirmed" && f.Status != "resolved" || kind == "appeals" && f.Status != "open" && f.Status != "closed") {
		return f, ErrInvalidRequest
	}
	if f.Kind != "" && (kind == "money" && f.Kind != "adjust" && f.Kind != "issue" || kind == "cases" && f.Kind != "binding_mismatch" && f.Kind != "integrity_mismatch" || kind == "appeals") {
		return f, ErrInvalidRequest
	}
	return f, nil
}
func sanitizeHistory(scope string, item any) any {
	if scope == "customer" {
		if c, ok := item.(domain.SecurityCase); ok {
			c.Evidence = ""
			c.Reviewer = ""
			c.ReviewReason = ""
			return c
		}
	}
	return item
}
func (s *Service) History(ctx context.Context, kind, scope, subject string, f domain.HistoryFilter) (domain.HistoryPage, error) {
	f, err := normalizeHistory(kind, scope, subject, f)
	if err != nil {
		return domain.HistoryPage{}, err
	}
	p, err := s.currentStore().History(ctx, kind, scope, subject, f)
	for i := range p.Items {
		p.Items[i] = sanitizeHistory(scope, p.Items[i])
	}
	return p, err
}
func (s *Service) StreamHistory(ctx context.Context, kind, scope, subject string, f domain.HistoryFilter, write func(any) error) error {
	f, err := normalizeHistory(kind, scope, subject, f)
	if err != nil {
		return err
	}
	return s.currentStore().StreamHistory(ctx, kind, scope, subject, f, func(item any) error { return write(sanitizeHistory(scope, item)) })
}
