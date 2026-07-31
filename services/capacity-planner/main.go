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

	"github.com/onlyarnav/nimbusdb/services/auth-service/auth"
	"github.com/onlyarnav/nimbusdb/services/capacity-planner/planner"
	pb "github.com/onlyarnav/nimbusdb/services/deployment-controller/proto"
	"github.com/onlyarnav/nimbusdb/services/observability/telemetry"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	slog.Info("starting nimbusdb capacity-planner microservice")

	grpcPort := os.Getenv("GRPC_PORT")
	if grpcPort == "" {
		grpcPort = "50059"
	}
	metricsPort := os.Getenv("METRICS_PORT")
	if metricsPort == "" {
		metricsPort = "8089"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	p := planner.NewPlanner()

	// Metrics HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"UP","service":"capacity-planner"}`))
	})
	mux.Handle("/metrics", telemetry.MetricsHandler())

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%s", metricsPort),
		Handler: mux,
	}

	go func() {
		slog.Info("capacity-planner HTTP/metrics server listening", "port", metricsPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("capacity-planner HTTP server failed", "error", err)
		}
	}()

	// gRPC server
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
	if err != nil {
		slog.Error("failed to listen for gRPC", "port", grpcPort, "error", err)
		os.Exit(1)
	}

	capRoleMap := map[string]string{
		"/proto.CapacityPlannerService/PredictCapacity": auth.RoleReadOnly,
	}

	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(auth.UnaryServerInterceptor(capRoleMap)))
	pb.RegisterCapacityPlannerServiceServer(grpcServer, p)

	go func() {
		slog.Info("capacity-planner gRPC server listening", "port", grpcPort)
		if err := grpcServer.Serve(lis); err != nil {
			slog.Error("capacity-planner gRPC server failed", "error", err)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down capacity-planner service gracefully")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	grpcServer.GracefulStop()
}
