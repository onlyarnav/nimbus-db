package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/onlyarnav/nimbusdb/services/metadata-service/config"
	"github.com/onlyarnav/nimbusdb/services/metadata-service/db"
	"github.com/onlyarnav/nimbusdb/services/metadata-service/handlers"
)

// Embed the SQL migrations directory into the binary
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

func main() {
	// Initialize structured logging (slog JSON handler to stdout)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	slog.Info("starting nimbusdb metadata service")

	// Load configuration
	cfg := config.Load()

	// Run schema migrations first
	slog.Info("running database migrations")
	if err := db.RunMigrations(cfg.DatabaseURL, migrationsFS, "migrations"); err != nil {
		slog.Error("failed to run database migrations", "error", err)
		os.Exit(1)
	}
	slog.Info("database migrations applied successfully")

	// Establish connection pool to database
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	slog.Info("connecting to database pool")
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	slog.Info("database connection pool established")

	// Register HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handlers.HealthHandler(pool))

	// Setup server with robust timeouts
	serverAddr := fmt.Sprintf(":%s", cfg.Port)
	srv := &http.Server{
		Addr:         serverAddr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Channel to listen for errors during server startup
	serverErrors := make(chan error, 1)

	// Start server in background
	go func() {
		slog.Info("http server listening", "address", serverAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	// Block until signal or server error
	select {
	case err := <-serverErrors:
		slog.Error("server error, shutting down", "error", err)
		os.Exit(1)
	case <-ctx.Done():
		slog.Info("shutdown signal received, initiating graceful shutdown")

		// Gracefully shut down server with 5s timeout
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("failed to gracefully shut down server", "error", err)
			_ = srv.Close()
		}
		slog.Info("http server shut down cleanly")
	}
}
