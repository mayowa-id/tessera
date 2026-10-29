package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"tessera/backend/internal/demo"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if os.Getenv("TESSERA_ALLOW_DEMO_SEED") != "true" || os.Getenv("VERCEL") != "" {
		return fmt.Errorf("demo seed is development-only: set TESSERA_ALLOW_DEMO_SEED=true outside Vercel")
	}
	if os.Getenv("DATABASE_URL") == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := demo.Seed(ctx, pool, os.Getenv("DEMO_PASSWORD")); err != nil {
		return err
	}
	fmt.Println("Demo organization ready. Sign in as manager@tessera.local or employee@tessera.local using DEMO_PASSWORD.")
	return nil
}
