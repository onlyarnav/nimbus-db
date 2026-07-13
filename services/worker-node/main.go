package main

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
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
	if interval <= 0 {
		interval = 5
	}
	slog.Info("node registered successfully", "node_id", nodeID, "heartbeat_interval_seconds", interval)

	// Setup debug HTTP server for failure simulation
	var paused atomic.Bool

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pause", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		paused.Store(true)
		slog.Warn("worker heartbeat loop PAUSED via debug endpoint")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("paused"))
	})
	mux.HandleFunc("/debug/resume", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		paused.Store(false)
		slog.Info("worker heartbeat loop RESUMED via debug endpoint")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("resumed"))
	})

	debugServer := &http.Server{
		Addr:    fmt.Sprintf(":%s", cfg.DebugPort),
		Handler: mux,
	}

	go func() {
		slog.Info("worker debug server listening", "port", cfg.DebugPort)
		if err := debugServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("worker debug server failed", "error", err)
		}
	}()

	// Start periodic heartbeat loop with random walk statistics
	var cpu float32 = 50.0
	var mem float32 = 50.0
	var disk float32 = 50.0

	randomWalk := func(val float32) float32 {
		// Generate random change between -2.0 and +2.0
		change := (rand.Float32() * 4) - 2
		newVal := val + change
		if newVal < 0 {
			newVal = 0
		}
		if newVal > 100 {
			newVal = 100
		}
		return newVal
	}

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if paused.Load() {
					slog.Info("heartbeat skipped (PAUSED)", "node_id", nodeID)
					continue
				}

				cpu = randomWalk(cpu)
				mem = randomWalk(mem)
				disk = randomWalk(disk)

				// Log that this statistics trace is simulated / fake
				slog.Info("sending simulated statistics heartbeat",
					"node_id", nodeID,
					"cpu_pct", cpu,
					"memory_pct", mem,
					"disk_pct", disk,
					"note", "synthetic metrics via random walk simulator",
				)

				hbCtx, hbCancel := context.WithTimeout(ctx, 3*time.Second)
				_, err := client.SendHeartbeat(hbCtx, &pb.SendHeartbeatRequest{
					NodeId:    nodeID,
					CpuPct:    cpu,
					MemoryPct: mem,
					DiskPct:   disk,
					Healthy:   true,
				})
				hbCancel()

				if err != nil {
					slog.Error("failed to send heartbeat to metadata service", "error", err)
				}
			}
		}
	}()

	// Block until shutdown signal is received
	slog.Info("worker node is running, waiting for signal...")
	<-ctx.Done()

	slog.Info("shutting down worker debug server")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	_ = debugServer.Shutdown(shutdownCtx)

	slog.Info("shutting down worker node gracefully")
}
