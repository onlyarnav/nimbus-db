package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/onlyarnav/nimbusdb/services/control-plane/config"
	pb "github.com/onlyarnav/nimbusdb/services/control-plane/proto/metadata"
)

func main() {
	// Initialize structured logging (slog JSON handler to stdout)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	slog.Info("starting nimbusdb control plane")

	cfg := config.Load()

	// Setup context that cancels on signal
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 1. Dial Metadata Service
	slog.Info("connecting to metadata service", "address", cfg.MetadataGRPCAddr)
	metaConn, err := grpc.DialContext(ctx, cfg.MetadataGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Error("failed to connect to metadata service", "error", err)
		os.Exit(1)
	}
	defer metaConn.Close()
	metadataClient := pb.NewMetadataServiceClient(metaConn)

	// 2. Dial Scheduler Service
	slog.Info("connecting to scheduler service", "address", cfg.SchedulerGRPCAddr)
	schedConn, err := grpc.DialContext(ctx, cfg.SchedulerGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Error("failed to connect to scheduler service", "error", err)
		os.Exit(1)
	}
	defer schedConn.Close()
	schedulerClient := pb.NewSchedulerServiceClient(schedConn)

	// 3. Initialize Orchestrator, Handlers, and Reconciler
	orchestrator := NewOrchestrator(metadataClient, schedulerClient)
	handlers := NewHandlers(metadataClient, orchestrator)

	// Timeout threshold of 30 seconds for stuck databases, poll interval of 5 seconds
	reconciler := NewReconciler(metadataClient, orchestrator, 30*time.Second)

	// 4. Start Reconciler loop in the background
	go reconciler.Start(ctx, 5*time.Second)

	// 5. Start HTTP REST Server
	mux := http.NewServeMux()
	handlers.RegisterRoutes(mux)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%s", cfg.HTTPPort),
		Handler: mux,
	}

	go func() {
		slog.Info("Control Plane REST server listening", "port", cfg.HTTPPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Control Plane REST server failed", "error", err)
		}
	}()

	// Block until shutdown signal is received
	slog.Info("control plane service is running, waiting for signal...")
	<-ctx.Done()

	slog.Info("shutting down HTTP REST server")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)

	slog.Info("control plane service shut down gracefully")
}
