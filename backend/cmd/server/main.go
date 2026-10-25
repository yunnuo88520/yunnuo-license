package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/buildinfo"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/config"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/httpapi"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

func main() {
	resetUsername := flag.String("reset-admin", "", "reset an existing administrator password (read one line from stdin)")
	resetReason := flag.String("reason", "", "reason for the administrator password reset")
	flag.Parse()
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	st, err := store.Open(ctx, cfg.DatabaseDriver, cfg.Database)
	if err != nil {
		log.Fatal(err)
	}
	migrationDir := filepath.Join("migrations")
	if err := st.Migrate(ctx, migrationDir); err != nil {
		_ = st.Close()
		log.Fatal(err)
	}

	adminCount, err := st.CountAdminUsers(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if err := cfg.PrepareInstallationKeys(adminCount > 0); err != nil {
		log.Fatal(err)
	}
	svc := service.New(st, cfg.CardPepper, cfg.DataKey).WithReleaseDir(cfg.ReleaseDir)
	defer svc.Close()
	if *resetUsername != "" {
		if err := resetAdministrator(ctx, svc, *resetUsername, *resetReason, os.Stdin); err != nil {
			log.Fatal(err)
		}
		log.Print("administrator password reset; previous sessions invalidated")
		return
	}
	if err := svc.EnsureAgentLoginCodes(ctx); err != nil {
		log.Fatal(err)
	}
	svc.StartExpiryReminderScheduler()
	server := &http.Server{
		Addr: cfg.Addr,
		Handler: httpapi.New(svc, cfg.PublicStaticDir, cfg.AdminStaticDir, cfg.AgentStaticDir).
			WithTrustedProxyHeaders(cfg.TrustProxyHeaders).
			WithAdminPath(cfg.AdminPath).
			WithSetupConfig(migrationDir, cfg.DatabaseConfigFile).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("yn-license %s listening on %s", buildinfo.Version, cfg.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatal(err)
	}
}
