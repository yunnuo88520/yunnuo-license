package service

import (
	"context"
	mysql "github.com/go-sql-driver/mysql"
	"net"
	"strconv"
	"strings"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

type SetupStatus struct {
	Initialized        bool   `json:"initialized"`
	DatabaseDriver     string `json:"database_driver"`
	SuggestedSQLiteDSN string `json:"suggested_sqlite_dsn,omitempty"`
}

type SetupInput struct {
	DatabaseHost     string `json:"database_host"`
	DatabasePort     int    `json:"database_port"`
	DatabaseName     string `json:"database_name"`
	DatabaseUser     string `json:"database_user"`
	DatabasePassword string `json:"database_password"`
	DatabasePath     string `json:"database_path"`
	AdminUsername    string `json:"admin_username"`
	AdminPassword    string `json:"admin_password"`
	AdminName        string `json:"admin_name"`
	DatabaseDriver   string `json:"database_driver"`
	DatabaseDSN      string `json:"database_dsn"`
}

type PersistDatabaseConfig func(driver, dsn string) error

func (s *Service) SetupStatus(ctx context.Context) (SetupStatus, error) {
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	return s.setupStatus(ctx)
}

func (s *Service) setupStatus(ctx context.Context) (SetupStatus, error) {
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

	status, err := s.setupStatus(ctx)
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
	if input.DatabaseDSN == "" {
		if input.DatabaseDriver == "sqlite" {
			if strings.TrimSpace(input.DatabasePath) != "" && !strings.ContainsAny(input.DatabasePath, "?#\x00") {
				input.DatabaseDSN = "file:" + input.DatabasePath + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
			}
		} else {
			if input.DatabasePort == 0 {
				input.DatabasePort = 3306
			}
			if input.DatabaseHost == "" || input.DatabaseName == "" || input.DatabaseUser == "" || input.DatabasePort < 1 || input.DatabasePort > 65535 {
				return ErrInvalidRequest
			}
			c := mysql.NewConfig()
			c.User = input.DatabaseUser
			c.Passwd = input.DatabasePassword
			c.Net = "tcp"
			c.Addr = net.JoinHostPort(input.DatabaseHost, strconv.Itoa(input.DatabasePort))
			c.DBName = input.DatabaseName
			c.MultiStatements = true
			c.Collation = "utf8mb4_unicode_ci"
			c.Params = map[string]string{"charset": "utf8mb4"}
			input.DatabaseDSN = c.FormatDSN()
		}
	}
	if strings.TrimSpace(input.DatabaseDSN) == "" {
		return ErrInvalidRequest
	}
	if input.AdminUsername == "" || len(input.AdminUsername) > 80 || len(input.AdminName) > 160 || len(strings.TrimSpace(input.AdminPassword)) < 8 || len(input.AdminPassword) > 256 {
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
