package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"labreserve.local/labreserve/internal/auth"
	"labreserve.local/labreserve/internal/config"
	"labreserve.local/labreserve/internal/database"
	"labreserve.local/labreserve/internal/web"
)

func main() {
	if err := run(); err != nil {
		log.Printf("LabReserve E2E test server: %v", err)
		os.Exit(1)
	}
}

func run() error {
	testNowValue := os.Getenv("LABRESERVE_TEST_NOW")
	if testNowValue == "" {
		return fmt.Errorf("LABRESERVE_TEST_NOW is required by the test-only server")
	}
	testNow, err := time.Parse(time.RFC3339Nano, testNowValue)
	if err != nil {
		return fmt.Errorf("LABRESERVE_TEST_NOW must be an RFC3339 instant")
	}
	now := func() time.Time { return testNow }

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	openCtx, openCancel := context.WithTimeout(context.Background(), 10*time.Second)
	pool, err := database.Open(openCtx, cfg.DatabaseURL)
	openCancel()
	if err != nil {
		return err
	}
	defer pool.Close()
	checkCtx, checkCancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = database.CheckSchema(checkCtx, pool)
	checkCancel()
	if err != nil {
		return err
	}

	store := database.NewStore(pool)
	authentication := auth.NewService(store, now)
	application, err := web.New(store, authentication, cfg, now)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr: cfg.ListenAddr, Handler: application.Handler(),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second,
	}
	log.Printf("LabReserve E2E test server listening on %s with controlled time", cfg.ListenAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("serve test HTTP: %w", err)
	}
	return nil
}
