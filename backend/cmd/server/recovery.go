package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strings"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
)

func resetAdministrator(ctx context.Context, svc *service.Service, username, reason string, input io.Reader) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("--reason is required")
	}
	scanner := bufio.NewScanner(io.LimitReader(input, 1024))
	if !scanner.Scan() || len(scanner.Text()) < 8 || len(scanner.Text()) > 256 {
		return errors.New("read a password of 8 to 256 bytes from stdin")
	}
	password := scanner.Text()
	users, err := svc.ListAdminUsers(ctx)
	if err != nil {
		return err
	}
	for _, user := range users {
		if user.Username == strings.TrimSpace(username) {
			return svc.ResetAccountPassword(ctx, "admin", user.ID, password, reason, "local-console", domain.AdminRoleSuperAdmin)
		}
	}
	return errors.New("administrator does not exist; complete /install for a new platform")
}
