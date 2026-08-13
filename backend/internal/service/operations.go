package service

import (
	"context"
	"strings"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

func (s *Service) ChangeAgentStatus(ctx context.Context, agentID, status, actorID string) (domain.Agent, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" || !validAgentStatus(status) {
		return domain.Agent{}, ErrInvalidRequest
	}
	agent, err := s.store.GetAgent(ctx, agentID)
	if err != nil {
		if store.IsNotFound(err) {
			return domain.Agent{}, ErrAgentNotFound
		}
		return domain.Agent{}, err
	}
	now := s.now()
	var disabledAt = agent.DisabledAt
	if status == domain.AgentDisabled {
		disabledAt = &now
	} else {
		disabledAt = nil
	}
	if err := s.store.UpdateAgentStatus(ctx, agentID, status, disabledAt, now); err != nil {
		return domain.Agent{}, err
	}
	agent.Status = status
	agent.DisabledAt = disabledAt
	agent.UpdatedAt = now
	loginCode, err := s.store.GetAgentLoginCode(ctx, agentID)
	if err != nil && !store.IsNotFound(err) {
		return domain.Agent{}, err
	}
	agent.LoginCode = loginCode
	_ = s.auditAdminAgent(ctx, actorID, agentID, "agent.status."+status, "success", "")
	return agent, nil
}

func (s *Service) ChangeAgentUserStatus(ctx context.Context, agentID, userID, status, actorID string) (domain.AgentUser, error) {
	agentID = strings.TrimSpace(agentID)
	userID = strings.TrimSpace(userID)
	if agentID == "" || userID == "" || (status != domain.AgentUserActive && status != domain.AgentUserDisabled) {
		return domain.AgentUser{}, ErrInvalidRequest
	}
	user, err := s.store.GetAgentUserByID(ctx, userID)
	if err != nil || user.AgentID != agentID {
		if err == nil || store.IsNotFound(err) {
			return domain.AgentUser{}, ErrAgentUserNotFound
		}
		return domain.AgentUser{}, err
	}
	now := s.now()
	if err := s.store.UpdateAgentUserStatus(ctx, agentID, userID, status, now); err != nil {
		if store.IsNotFound(err) {
			return domain.AgentUser{}, ErrAgentUserNotFound
		}
		return domain.AgentUser{}, err
	}
	user.Status = status
	user.UpdatedAt = now
	_ = s.auditAdminAgent(ctx, actorID, agentID, "agent_user.status."+status, "success", "")
	return user, nil
}

type AuditLogFilter struct {
	ActorType string
	Result    string
	Action    string
	Limit     int
}

func (s *Service) ListAuditLogs(ctx context.Context, filter AuditLogFilter) ([]domain.AuditLog, error) {
	filter.ActorType = strings.TrimSpace(filter.ActorType)
	filter.Result = strings.TrimSpace(filter.Result)
	filter.Action = strings.TrimSpace(filter.Action)
	if filter.ActorType != "" && filter.ActorType != "admin" && filter.ActorType != "agent" && filter.ActorType != "client" {
		return nil, ErrInvalidRequest
	}
	if filter.Result != "" && filter.Result != "success" && filter.Result != "failed" {
		return nil, ErrInvalidRequest
	}
	if filter.Limit <= 0 {
		filter.Limit = 100
	}
	if filter.Limit > 200 {
		filter.Limit = 200
	}
	return s.store.ListAuditLogs(ctx, filter.ActorType, filter.Result, filter.Action, filter.Limit)
}

func validAgentStatus(status string) bool {
	switch status {
	case domain.AgentActive, domain.AgentSuspended, domain.AgentDisabled:
		return true
	default:
		return false
	}
}
