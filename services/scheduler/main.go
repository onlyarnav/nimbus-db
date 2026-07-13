package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/onlyarnav/nimbusdb/services/scheduler/config"
	schedgrpc "github.com/onlyarnav/nimbusdb/services/scheduler/grpc"
	pb "github.com/onlyarnav/nimbusdb/services/scheduler/proto"
)

func main() {
	// Initialize structured logging (slog JSON handler to stdout)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	slog.Info("starting nimbusdb scheduler service")

	cfg := config.Load()

	// Context for graceful shutdowns
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Dial Metadata Service gRPC server
	slog.Info("connecting to metadata service", "address", cfg.MetadataGRPCAddr)
	metaConn, err := grpc.DialContext(ctx, cfg.MetadataGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Error("failed to connect to metadata service", "error", err)
		os.Exit(1)
	}
	defer metaConn.Close()

	metaClient := pb.NewMetadataServiceClient(metaConn)

	// Setup Scheduler gRPC listener
	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.SchedulerPort))
	if err != nil {
		slog.Error("failed to listen on scheduler port", "error", err)
		os.Exit(1)
	}

	gSrv := grpc.NewServer()
	pb.RegisterSchedulerServiceServer(gSrv, schedgrpc.NewServer(metaClient))

	serverErrors := make(chan error, 1)
	go func() {
		slog.Info("scheduler grpc server listening", "address", listener.Addr().String())
		if err := gSrv.Serve(listener); err != nil {
			serverErrors <- err
		}
	}()

	select {
	case err := <-serverErrors:
		slog.Error("scheduler server failure", "error", err)
		os.Exit(1)
	case <-ctx.Done():
		slog.Info("shutdown signal received, shutting down scheduler gracefully")
		gSrv.GracefulStop()
		slog.Info("scheduler shut down cleanly")
	}
}
