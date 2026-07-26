package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/onlyarnav/nimbusdb/services/observability/telemetry"
	"github.com/onlyarnav/nimbusdb/services/sla-monitor/monitor"
	pb "github.com/onlyarnav/nimbusdb/services/sla-monitor/proto"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	slog.Info("starting nimbusdb sla-monitor service")

	grpcPort := os.Getenv("GRPC_PORT")
	if grpcPort == "" {
		grpcPort = "50060"
	}
	metricsPort := os.Getenv("METRICS_PORT")
	if metricsPort == "" {
		metricsPort = "8090"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mon := monitor.NewMonitor()

	// Metrics HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"UP","service":"sla-monitor"}`))
	})
	mux.Handle("/metrics", telemetry.MetricsHandler())

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%s", metricsPort),
		Handler: mux,
	}

	go func() {
		slog.Info("sla-monitor HTTP/metrics server listening", "port", metricsPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("sla-monitor HTTP server failed", "error", err)
		}
	}()

	// gRPC server
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
	if err != nil {
		slog.Error("failed to listen for gRPC", "port", grpcPort, "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterSLAMonitorServiceServer(grpcServer, mon)

	go func() {
		slog.Info("sla-monitor gRPC server listening", "port", grpcPort)
		if err := grpcServer.Serve(lis); err != nil {
			slog.Error("sla-monitor gRPC server failed", "error", err)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down sla-monitor service gracefully")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	grpcServer.GracefulStop()
}
