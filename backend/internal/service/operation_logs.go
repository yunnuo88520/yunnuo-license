package service

import (
	"context"
	"strings"
	"time"

	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

type AuditActor struct{ Type, ID, Role, RequestID, ClientIP string }
type auditActorKey struct{}

func WithAuditActor(ctx context.Context, a *AuditActor) context.Context {
	return context.WithValue(ctx, auditActorKey{}, a)
}
func ActorFrom(ctx context.Context) *AuditActor {
	a, _ := ctx.Value(auditActorKey{}).(*AuditActor)
	return a
}

type OperationTicket struct {
	st  *store.Store
	Log domain.OperationLog
}

func (s *Service) BeginOperation(ctx context.Context, requestID, category, method, route, ip string, a *AuditActor) (OperationTicket, error) {
	l := domain.OperationLog{ID: yncrypto.NewID("op"), RequestID: requestID, Category: category, ActorType: a.Type, ActorID: a.ID, Role: a.Role, Method: method, Route: route, ClientIP: ip, Result: "pending", CreatedAt: s.now()}
	ticket := OperationTicket{st: s.currentStore(), Log: l}
	return ticket, ticket.st.BeginOperation(ctx, l)
}
func (s *Service) FinishOperation(ctx context.Context, t OperationTicket, a *AuditActor, status int, code string) error {
	l := t.Log
	l.ActorType = a.Type
	l.ActorID = a.ID
	l.Role = a.Role
	l.HTTPStatus = status
	l.ErrorCode = code
	l.Result = "success"
	if status >= 400 {
		l.Result = "failed"
	}
	if err := t.st.FinishOperation(ctx, l); err != nil {
		return err
	}
	if t.st != s.currentStore() {
		return s.currentStore().ImportOperation(ctx, l)
	}
	return nil
}

type OperationFilter struct {
	Category, Actor, Result, Query, From, To string
	Page, PageSize                           int
}
type OperationPage struct {
	Items    []domain.OperationLog `json:"items"`
	Total    int                   `json:"total"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"page_size"`
}

func (s *Service) OperationLogs(ctx context.Context, f OperationFilter) (OperationPage, error) {
	if len(f.Query) > 100 || len(f.Category) > 96 || len(f.Actor) > 32 {
		return OperationPage{}, ErrInvalidRequest
	}
	if f.Result != "" && f.Result != "success" && f.Result != "failed" && f.Result != "pending" {
		return OperationPage{}, ErrInvalidRequest
	}
	for _, value := range []string{f.From, f.To} {
		if value != "" {
			if _, err := time.Parse(time.RFC3339, value); err != nil {
				return OperationPage{}, ErrInvalidRequest
			}
		}
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 30
	}
	if f.PageSize > 5000 {
		f.PageSize = 5000
	}
	items, total, err := s.currentStore().Operations(ctx, f.Category, f.Actor, f.Result, strings.TrimSpace(f.Query), f.From, f.To, f.Page, f.PageSize)
	return OperationPage{Items: items, Total: total, Page: f.Page, PageSize: f.PageSize}, err
}
