package service

import (
	"context"
	"strings"
	"time"
)

type PublicAgentInput struct {
	AgentNo         string `json:"agent_no"`
	ApplicationCode string `json:"application_code"`
}
type PublicAgentProduct struct {
	Name string `json:"name"`
	Code string `json:"code"`
}
type PublicAgentView struct {
	AgentNo      string               `json:"agent_no"`
	Name         string               `json:"name"`
	Status       string               `json:"status"`
	IsAuthorized bool                 `json:"is_authorized"`
	Applications []PublicAgentProduct `json:"applications"`
	ServerTime   time.Time            `json:"server_time"`
}

func (s *Service) PublicAgentQuery(ctx context.Context, in PublicAgentInput) (PublicAgentView, error) {
	var v PublicAgentView
	in.AgentNo = strings.ToUpper(strings.TrimSpace(in.AgentNo))
	in.ApplicationCode = strings.ToUpper(strings.TrimSpace(in.ApplicationCode))
	if len(in.AgentNo) < 8 || len(in.AgentNo) > 80 || len(in.ApplicationCode) > 32 {
		return v, ErrInvalidRequest
	}
	a, err := s.currentStore().GetAgentByNo(ctx, in.AgentNo)
	if err != nil {
		return v, ErrAgentNotFound
	}
	v = PublicAgentView{AgentNo: a.AgentNo, Name: a.Name, Status: a.Status, ServerTime: s.now(), Applications: []PublicAgentProduct{}}
	if a.Status != "active" {
		return v, nil
	}
	quotes, err := s.AgentPackages(ctx, a.ID)
	if err != nil {
		return v, err
	}
	seen := map[string]bool{}
	for _, q := range quotes {
		if seen[q.Package.ProductID] {
			continue
		}
		p, err := s.currentStore().GetProduct(ctx, q.Package.ProductID)
		if err != nil {
			return v, err
		}
		if in.ApplicationCode != "" && p.Code != in.ApplicationCode {
			continue
		}
		seen[p.ID] = true
		v.Applications = append(v.Applications, PublicAgentProduct{Name: p.Name, Code: p.Code})
	}
	v.IsAuthorized = len(v.Applications) > 0
	return v, nil
}
