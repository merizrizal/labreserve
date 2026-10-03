package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"labreserve.local/labreserve/internal/auth"
	"labreserve.local/labreserve/internal/config"
	"labreserve.local/labreserve/internal/database"
	"labreserve.local/labreserve/internal/web"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Printf("LabReserve: %v", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	command := "serve"
	if len(args) > 0 {
		command = args[0]
	}
	switch command {
	case "serve":
		return serve()
	case "migrate":
		return migrate()
	case "seed":
		return seed()
	default:
		return fmt.Errorf("unknown command %q (use serve, migrate, or seed)", command)
	}
}

func serve() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	cancel()
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := database.CheckSchema(context.Background(), pool); err != nil {
		return err
	}

	store := database.NewStore(pool)
	authentication := auth.NewService(store, time.Now)
	application, err := web.New(store, authentication, cfg, time.Now)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr: cfg.ListenAddr, Handler: application.Handler(),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serveErr := make(chan error, 1)
	go func() {
		log.Printf("LabReserve listening on %s", cfg.ListenAddr)
		serveErr <- server.ListenAndServe()
	}()
	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shut down HTTP server: %w", err)
		}
		return nil
	}
}

func migrate() error {
	databaseURL, err := requiredEnvironment("MIGRATION_DATABASE_URL")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool); err != nil {
		return err
	}
	log.Print("database migrations applied")
	return nil
}

func seed() error {
	databaseURL, err := requiredEnvironment("MIGRATION_DATABASE_URL")
	if err != nil {
		return err
	}
	passwordEnvironment := map[string]string{
		"alex@example.test":   "SEED_ALEX_PASSWORD",
		"sam@example.test":    "SEED_SAM_PASSWORD",
		"jordan@example.test": "SEED_JORDAN_PASSWORD",
	}
	passwordHashes := make(map[string]string, len(passwordEnvironment))
	for login, key := range passwordEnvironment {
		password, err := requiredEnvironment(key)
		if err != nil {
			return err
		}
		hash, err := auth.HashPassword(password)
		if err != nil {
			return fmt.Errorf("hash a demo account password")
		}
		passwordHashes[login] = hash
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := database.CheckSchema(ctx, pool); err != nil {
		return err
	}
	if err := database.Seed(ctx, pool, passwordHashes); err != nil {
		return err
	}
	log.Print("fictional demo accounts and resources seeded without replacing existing rows")
	return nil
}

func requiredEnvironment(key string) (string, error) {
	value, exists := os.LookupEnv(key)
	if !exists || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}
