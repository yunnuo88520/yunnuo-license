package httpapi

import (
	"context"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
	"testing"
)

func TestPackageCostsMustBeExplicitAndZeroIsAllowed(t *testing.T) {
	ctx := context.Background()
	s := newHTTPTestService(t)
	if _, _, err := s.EnsureBootstrapAdmin(ctx, "root", "secret123", "Root"); err != nil {
		t.Fatal(err)
	}
	h := New(s, "", "", "").Handler()
	root := loginAdmin(t, h, "root", "secret123")
	p, err := s.CreateProduct(ctx, service.CreateProductInput{Name: "Costs", Code: "CST", BindMode: domain.BindDomain})
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"product_id": p.ID, "name": "Trial", "duration_days": 30}
	performJSONStatus(t, h, "POST", "/admin/packages", root, input, 400)
	input["base_cost"] = 0
	var pkg domain.CardPackage
	decodeData(t, performJSON(t, h, "POST", "/admin/packages", root, input), &pkg)
	if pkg.BaseCost != 0 || !pkg.Enabled {
		t.Fatal(pkg)
	}
	performJSONStatus(t, h, "POST", "/admin/packages/"+pkg.ID, root, map[string]any{"enabled": true}, 400)
	performJSONStatus(t, h, "POST", "/admin/packages/"+pkg.ID, root, map[string]any{"base_cost": 500}, 400)
}
