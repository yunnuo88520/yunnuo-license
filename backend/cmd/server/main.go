package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/config"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/httpapi"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/store"
)

func main() {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	st, err := store.Open(ctx, cfg.DatabaseDriver, cfg.Database)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	migrationDir := filepath.Join("migrations")
	if err := st.Migrate(ctx, migrationDir); err != nil {
		log.Fatal(err)
	}

	svc := service.New(st, cfg.CardPepper, cfg.DataKey)
	if err := svc.EnsureAgentLoginCodes(ctx); err != nil {
		log.Fatal(err)
	}
	admin, created, err := svc.EnsureBootstrapAdmin(ctx, cfg.AdminUsername, cfg.AdminPassword, cfg.AdminName)
	if err != nil {
		log.Fatal(err)
	}
	if created {
		log.Printf("bootstrap admin created: %s", admin.Username)
		if cfg.AdminPassword == "admin123" {
			log.Printf("warning: bootstrap admin uses the development password; change it after login or set YN_ADMIN_PASSWORD before first start")
		}
	}
	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           httpapi.New(svc, cfg.PublicStaticDir, cfg.AdminStaticDir, cfg.AgentStaticDir).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("yn-license backend listening on %s", cfg.Addr)
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
