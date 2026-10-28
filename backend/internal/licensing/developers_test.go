package licensing_test

import (
	"context"
	core "github.com/yunnuo88520/yunnuo-license/backend/internal/licensing"
	"testing"
)

func TestDeveloperReassignmentImmediatelyWithdrawsOldScope(t *testing.T) {
	s, _, root, _ := coreFixture(t)
	developer := func(id string) context.Context {
		return core.BindPrincipal(context.Background(), core.Principal{Actor: core.Actor{Kind: "developer", ID: id, Role: "developer"}})
	}
	a, e := s.CreateApplication(developer("alice"), core.CreateApplicationInput{Name: "Ebox", Group: "Business"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.GetApplication(developer("bob"), a.AppID); e == nil {
		t.Fatal("foreign developer read accepted")
	}
	if _, e = s.AssignDeveloper(developer("alice"), a.AppID, "bob", "unauthorized transfer"); e == nil {
		t.Fatal("developer transferred ownership")
	}
	if _, e = s.AssignDeveloper(root, a.AppID, "bob", "operator reassignment"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.GetApplication(developer("alice"), a.AppID); e == nil {
		t.Fatal("old developer retained access")
	}
	apps, e := s.ListApplications(developer("alice"))
	if e != nil || len(apps) != 0 {
		t.Fatal("old directory still visible", e)
	}
	app, e := s.GetApplication(developer("bob"), a.AppID)
	if e != nil || app.Group != "Business" {
		t.Fatal("new owner could not read", e)
	}
	if _, e = s.SavePlan(developer("alice"), a.AppID, "", core.PlanInput{Name: "foreign", Terms: core.Terms{Model: "perpetual", BindingTypes: []string{"installation"}}}); e == nil {
		t.Fatal("old developer retained writes")
	}
	if _, e = s.AuditIntegrity(root, a.AppID); e != nil {
		t.Fatal(e)
	}
}
