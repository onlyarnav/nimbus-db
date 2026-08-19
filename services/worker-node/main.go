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

	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/onlyarnav/nimbusdb/services/auth-service/auth"
	"github.com/onlyarnav/nimbusdb/services/observability/telemetry"
	"github.com/onlyarnav/nimbusdb/services/worker-node/agent"
	"github.com/onlyarnav/nimbusdb/services/worker-node/config"
	pb "github.com/onlyarnav/nimbusdb/services/worker-node/proto"
	pbAgent "github.com/onlyarnav/nimbusdb/services/worker-node/proto/nodeagent"

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
	conn, err := grpc.DialContext(ctx, cfg.MetadataGRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(auth.UnaryClientInterceptor("worker-node", auth.RoleOperator)),
	)
	if err != nil {
		slog.Error("failed to connect to metadata service", "error", err)
		os.Exit(1)
	}
	defer conn.Close()

	client := pb.NewMetadataServiceClient(conn)

	// Call RegisterNode on startup with retry for cold start resilience
	slog.Info("registering node with metadata service", "cluster_id", cfg.ClusterID, "hostname", cfg.Hostname)

	var nodeID string
	var interval int32 = 5

	for {
		regCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		res, err := client.RegisterNode(regCtx, &pb.RegisterNodeRequest{
			ClusterId: cfg.ClusterID,
			Hostname:  cfg.Hostname,
		})
		cancel()
		if err == nil {
			nodeID = res.GetNodeId()
			if res.GetHeartbeatIntervalSeconds() > 0 {
				interval = res.GetHeartbeatIntervalSeconds()
			}
			break
		}

		st, ok := status.FromError(err)
		if ok && st.Code() == codes.AlreadyExists {
			nodesCtx, nodesCancel := context.WithTimeout(ctx, 5*time.Second)
			nodesRes, getErr := client.GetNodes(nodesCtx, &pb.GetNodesRequest{ClusterId: cfg.ClusterID})
			nodesCancel()
			if getErr == nil {
				for _, n := range nodesRes.GetNodes() {
					if n.GetHostname() == cfg.Hostname {
						nodeID = n.GetId()
						slog.Info("re-attached to existing node registration", "node_id", nodeID, "hostname", cfg.Hostname)
						break
					}
				}
				if nodeID != "" {
					break
				}
			}
		}

		slog.Warn("retrying node registration with metadata service", "error", err)
		select {
		case <-ctx.Done():
			slog.Error("registration aborted on shutdown")
			os.Exit(1)
		case <-time.After(2 * time.Second):
		}
	}

	slog.Info("node registered successfully", "node_id", nodeID, "heartbeat_interval_seconds", interval)


	// Setup NodeAgent Server
	agentServer := agent.NewServer("data", cfg.Hostname)

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
	mux.HandleFunc("/debug/inject-failure", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		attemptsStr := r.URL.Query().Get("attempts")
		hangStr := r.URL.Query().Get("hang")
		var attemptsVal int
		var hangVal int
		if attemptsStr != "" {
			fmt.Sscanf(attemptsStr, "%d", &attemptsVal)
			atomic.StoreInt32(&agentServer.FailAttempts, int32(attemptsVal))
		}
		if hangStr != "" {
			fmt.Sscanf(hangStr, "%d", &hangVal)
			atomic.StoreInt32(&agentServer.HangAttempts, int32(hangVal))
		}
		slog.Warn("injected simulated database provisioning parameters", "fail_attempts", attemptsVal, "hang_attempts", hangVal)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("injected"))
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"UP","service":"worker-node"}`))
	})
	mux.Handle("/metrics", telemetry.MetricsHandler())


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

	// Start NodeAgent gRPC server
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.NodeAgentPort))
	if err != nil {
		slog.Error("failed to listen for NodeAgent gRPC", "error", err)
		os.Exit(1)
	}

	nodeRoleMap := map[string]string{
		"/proto.NodeAgent/CreateDatabase":  auth.RoleOperator,
		"/proto.NodeAgent/DeleteDatabase":  auth.RoleOperator,
		"/proto.NodeAgent/BackupDatabase":  auth.RoleOperator,
		"/proto.NodeAgent/RestoreDatabase": auth.RoleOperator,
		"/proto.NodeAgent/InsertVector":    auth.RoleOperator,
		"/proto.NodeAgent/SearchVector":    auth.RoleReadOnly,
		"/proto.NodeAgent/DrainNode":       auth.RoleAdmin,
	}

	grpcAgentServer := grpc.NewServer(grpc.UnaryInterceptor(auth.UnaryServerInterceptor(nodeRoleMap)))
	pbAgent.RegisterNodeAgentServer(grpcAgentServer, agentServer)

	go func() {
		slog.Info("NodeAgent gRPC server listening", "port", cfg.NodeAgentPort)
		if err := grpcAgentServer.Serve(lis); err != nil {
			slog.Error("NodeAgent gRPC server failed", "error", err)
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

				telemetry.CPUUsagePercent.WithLabelValues(nodeID, cfg.Hostname).Set(float64(cpu))
				telemetry.MemoryUsagePercent.WithLabelValues(nodeID, cfg.Hostname).Set(float64(mem))
				telemetry.DiskUsagePercent.WithLabelValues(nodeID, cfg.Hostname).Set(float64(disk))

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

	slog.Info("shutting down NodeAgent gRPC server")
	grpcAgentServer.GracefulStop()

	slog.Info("shutting down worker node gracefully")
}
