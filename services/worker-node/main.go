package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/onlyarnav/nimbusdb/services/worker-node/config"
	pb "github.com/onlyarnav/nimbusdb/services/worker-node/proto"
)

func main() {
	// Initialize structured logging (slog JSON handler to stdout)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	slog.Info("starting nimbusdb worker node")

	cfg := config.Load()

	// Setup context that cancels on signal
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	slog.Info("connecting to metadata service", "address", cfg.MetadataGRPCAddr)
	conn, err := grpc.DialContext(ctx, cfg.MetadataGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Error("failed to connect to metadata service", "error", err)
		os.Exit(1)
	}
	defer conn.Close()

	client := pb.NewMetadataServiceClient(conn)

	// Call RegisterNode on startup
	slog.Info("registering node with metadata service", "cluster_id", cfg.ClusterID, "hostname", cfg.Hostname)

	regCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	res, err := client.RegisterNode(regCtx, &pb.RegisterNodeRequest{
		ClusterId: cfg.ClusterID,
		Hostname:  cfg.Hostname,
	})
	if err != nil {
		slog.Error("failed to register node with metadata service", "error", err)
		os.Exit(1)
	}

	nodeID := res.GetNodeId()
	interval := res.GetHeartbeatIntervalSeconds()
	slog.Info("node registered successfully", "node_id", nodeID, "heartbeat_interval_seconds", interval)

	// Block until shutdown signal is received
	slog.Info("worker node is running, waiting for signal...")
	<-ctx.Done()

	slog.Info("shutting down worker node gracefully")
}
