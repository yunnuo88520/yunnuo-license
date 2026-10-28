package licensing

import (
	"context"
	"strings"
)

// AssignDeveloper is a platform administrator action. Developer sessions
// cannot transfer an application and the DAL rechecks ownership on each view.
func (s *Service) AssignDeveloper(ctx context.Context, id, developerID, reason string) (Application, error) {
	p, e := PrincipalFrom(ctx)
	if e != nil {
		return Application{}, e
	}
	if p.Actor.Kind != "admin" || !privileged(p, true) {
		return Application{}, forbidden("ROLE_FORBIDDEN", "只有管理员可以分配开发者")
	}
	if strings.TrimSpace(reason) == "" || len(reason) > 500 || len(developerID) > 96 {
		return Application{}, invalid("请选择开发者并填写分配原因")
	}
	scoped, _, repo, e := s.applicationContext(ctx, id, true)
	if e != nil {
		return Application{}, e
	}
	var app Application
	e = repo.Update(scoped, func(tx Transaction) error {
		var e error
		app, e = tx.Application(scoped)
		if e != nil {
			return e
		}
		old := app.DeveloperID
		app.DeveloperID = developerID
		app.UpdatedAt = s.now()
		if e = tx.SaveApplication(scoped, app); e != nil {
			return e
		}
		return s.append(scoped, tx, "developer_assigned", id, map[string]any{"previous_developer_id": old, "developer_id": developerID, "reason": reason})
	})
	app.SecretHash = ""
	return app, e
}
