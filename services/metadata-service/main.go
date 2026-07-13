package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/onlyarnav/nimbusdb/services/metadata-service/config"
	"github.com/onlyarnav/nimbusdb/services/metadata-service/db"
	grpcserver "github.com/onlyarnav/nimbusdb/services/metadata-service/grpc"
	"github.com/onlyarnav/nimbusdb/services/metadata-service/handlers"
	pb "github.com/onlyarnav/nimbusdb/services/metadata-service/proto"
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

	// Initialize and start the Health Manager background daemon (checks every 2 seconds)
	hm := db.NewHealthManager(pool, 2*time.Second)
	go hm.Start(ctx)

	// Register HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handlers.HealthHandler(pool))

	// Setup HTTP server with robust timeouts
	serverAddr := fmt.Sprintf(":%s", cfg.Port)
	srv := &http.Server{
		Addr:         serverAddr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Setup gRPC server
	grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.GRPCPort))
	if err != nil {
		slog.Error("failed to listen on grpc port", "error", err)
		os.Exit(1)
	}

	gSrv := grpc.NewServer()
	pb.RegisterMetadataServiceServer(gSrv, grpcserver.NewServer(pool))

	// Channel to listen for errors during server startup
	serverErrors := make(chan error, 2)

	// Start HTTP server in background
	go func() {
		slog.Info("http server listening", "address", serverAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- fmt.Errorf("http server error: %w", err)
		}
	}()

	// Start gRPC server in background
	go func() {
		slog.Info("grpc server listening", "address", grpcListener.Addr().String())
		if err := gSrv.Serve(grpcListener); err != nil {
			serverErrors <- fmt.Errorf("grpc server error: %w", err)
		}
	}()

	// Block until signal or server error
	select {
	case err := <-serverErrors:
		slog.Error("server error, shutting down", "error", err)
		os.Exit(1)
	case <-ctx.Done():
		slog.Info("shutdown signal received, initiating graceful shutdown")

		// Gracefully shut down HTTP server with 5s timeout
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("failed to gracefully shut down http server", "error", err)
			_ = srv.Close()
		}
		slog.Info("http server shut down cleanly")

		// Gracefully shut down gRPC server
		gSrv.GracefulStop()
		slog.Info("grpc server shut down cleanly")
	}
}
