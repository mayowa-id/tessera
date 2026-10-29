package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"tessera/backend/internal/auth"
	"tessera/backend/internal/platform"
	"tessera/backend/internal/staff"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("API stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := platform.LoadConfig()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	// Keep each server instance's connection budget small for serverless deployments.
	poolConfig.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return err
	}
	defer pool.Close()
	identity, err := auth.NewHandler(auth.NewPostgresStore(pool), cfg.AppOrigin, cfg.SecureCookies, logger)
	if err != nil {
		return err
	}
	staffAdmin := staff.NewHandler(staff.NewPostgresStore(pool), identity.RequireManager, logger)
	server := &http.Server{Addr: cfg.Address, Handler: platform.Router(pool, logger, identity.Register, staffAdmin.Register), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	result := make(chan error, 1)
	go func() { logger.Info("API listening", "address", cfg.Address); result <- server.ListenAndServe() }()
	select {
	case err := <-result:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
	return nil
}
