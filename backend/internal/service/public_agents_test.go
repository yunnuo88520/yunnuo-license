package service

import (
	"context"
	"encoding/json"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"strings"
	"testing"
)

func TestPublicAgentVerificationUsesCurrentGrantAndExcludesSecrets(t *testing.T) {
	ctx := context.Background()
	s := newTestService(t)
	p := createTestProduct(t, ctx, s, domain.ConflictReject)
	a, err := s.CreateAgent(ctx, CreateAgentInput{Name: "官方代理", Phone: "13812345678", Email: "private@example.com", Remark: "private-remark"})
	if err != nil {
		t.Fatal(err)
	}
	pkg := packageFixture(t, s, a.ID, p.ID, 900, false)
	view, err := s.PublicAgentQuery(ctx, PublicAgentInput{AgentNo: a.AgentNo, ApplicationID: p.ID})
	if err != nil || !view.IsAuthorized || len(view.Applications) != 1 || view.Applications[0].ID != p.ID {
		t.Fatal(view, err)
	}
	raw, _ := json.Marshal(view)
	for _, secret := range []string{a.ID, a.LoginCode, a.Phone, a.Email, a.Remark, p.PublicKeyPEM, "base_cost", "unit_cost", "balance"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("public agent query exposed private field %q", secret)
		}
	}
	view, err = s.PublicAgentQuery(ctx, PublicAgentInput{AgentNo: a.AgentNo, ApplicationID: "prod_other"})
	if err != nil || view.IsAuthorized || len(view.Applications) != 0 {
		t.Fatal("ungranted application was approved", view, err)
	}
	if _, err = s.ChangeAgentStatus(ctx, a.ID, domain.AgentSuspended, "admin"); err != nil {
		t.Fatal(err)
	}
	view, err = s.PublicAgentQuery(ctx, PublicAgentInput{AgentNo: a.AgentNo})
	if err != nil || view.IsAuthorized || view.Status != domain.AgentSuspended {
		t.Fatal("suspended agent approved", view, err)
	}
	if _, err = s.ChangeAgentStatus(ctx, a.ID, domain.AgentActive, "admin"); err != nil {
		t.Fatal(err)
	}
	if err = s.GrantPackages(ctx, GrantPackagesInput{AgentID: a.ID, PackageID: pkg.ID, Enabled: false, MaxBatchQuantity: 10}, "admin"); err != nil {
		t.Fatal(err)
	}
	view, err = s.PublicAgentQuery(ctx, PublicAgentInput{AgentNo: a.AgentNo})
	if err != nil || view.IsAuthorized {
		t.Fatal("revoked grant approved", view, err)
	}
	_, err = s.PublicAgentQuery(ctx, PublicAgentInput{AgentNo: a.LoginCode})
	assertCode(t, err, "AGENT_NOT_FOUND")
}
