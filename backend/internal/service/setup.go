package service

import (
	"context"
	"strings"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

type SetupStatus struct {
	Initialized        bool   `json:"initialized"`
	DatabaseDriver     string `json:"database_driver"`
	SuggestedSQLiteDSN string `json:"suggested_sqlite_dsn,omitempty"`
}

type SetupInput struct {
	AdminUsername  string `json:"admin_username"`
	AdminPassword  string `json:"admin_password"`
	AdminName      string `json:"admin_name"`
	DatabaseDriver string `json:"database_driver"`
	DatabaseDSN    string `json:"database_dsn"`
}

type PersistDatabaseConfig func(driver, dsn string) error

func (s *Service) SetupStatus(ctx context.Context) (SetupStatus, error) {
	st := s.currentStore()
	count, err := st.CountAdminUsers(ctx)
	if err != nil {
		return SetupStatus{}, err
	}
	return SetupStatus{Initialized: count > 0, DatabaseDriver: st.Driver()}, nil
}

func (s *Service) Initialize(ctx context.Context, input SetupInput, migrationDir string, persist PersistDatabaseConfig) error {
	s.setupMu.Lock()
	defer s.setupMu.Unlock()

	status, err := s.SetupStatus(ctx)
	if err != nil {
		return err
	}
	if status.Initialized {
		return ErrPermissionDenied
	}
	input.AdminUsername = strings.TrimSpace(input.AdminUsername)
	input.AdminName = strings.TrimSpace(input.AdminName)
	if input.AdminName == "" {
		input.AdminName = "系统管理员"
	}
	if input.DatabaseDriver != "sqlite" && input.DatabaseDriver != "mysql" {
		return ErrInvalidRequest
	}
	if strings.TrimSpace(input.DatabaseDSN) == "" {
		return ErrInvalidRequest
	}
	if input.AdminUsername == "" || len(strings.TrimSpace(input.AdminPassword)) < 8 {
		return ErrInvalidRequest
	}

	target, err := store.Open(ctx, input.DatabaseDriver, input.DatabaseDSN)
	if err != nil {
		return ErrDatabaseConnection
	}
	if err := target.Migrate(ctx, migrationDir); err != nil {
		_ = target.Close()
		return ErrDatabaseMigration
	}
	defer func() {
		if target != nil {
			_ = target.Close()
		}
	}()
	count, err := target.CountAdminUsers(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrSetupDatabaseInitialized
	}
	targetService := New(target, s.cardPepper, s.dataKey)
	if err := targetService.EnsureAgentLoginCodes(ctx); err != nil {
		return err
	}
	admin, created, err := targetService.EnsureBootstrapAdmin(ctx, input.AdminUsername, input.AdminPassword, input.AdminName)
	if err != nil {
		return err
	}
	if !created {
		return ErrSetupDatabaseInitialized
	}
	if persist != nil {
		if err := persist(input.DatabaseDriver, input.DatabaseDSN); err != nil {
			_ = target.DeleteAdminUser(ctx, admin.ID)
			return err
		}
	}
	s.replaceStore(target)
	target = nil
	return nil
}
