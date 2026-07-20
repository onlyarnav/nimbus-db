package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/onlyarnav/nimbusdb/services/control-plane/proto/metadata"
	pbAgent "github.com/onlyarnav/nimbusdb/services/control-plane/proto/nodeagent"
)

// Orchestrator coordinates provisioning and placement tasks.
type Orchestrator struct {
	metadataClient  pb.MetadataServiceClient
	schedulerClient pb.SchedulerServiceClient
}

// NewOrchestrator creates a new Orchestrator instance.
func NewOrchestrator(mc pb.MetadataServiceClient, sc pb.SchedulerServiceClient) *Orchestrator {
	return &Orchestrator{
		metadataClient:  mc,
		schedulerClient: sc,
	}
}

// ProvisionDatabase coordinates the retryable creation state machine.
func (o *Orchestrator) ProvisionDatabase(ctx context.Context, dbID string, name string, clusterID string) {
	slog.Info("starting provisioning flow for database", "database_id", dbID, "name", name, "cluster_id", clusterID)

	var lastErr error
	attempts := 0
	maxAttempts := 3

	for attempts < maxAttempts {
		attempts++
		slog.Info("provisioning attempt started", "database_id", dbID, "attempt", attempts, "max_attempts", maxAttempts)

		// 1. Call Scheduler to pick a placement node
		schedRes, err := o.schedulerClient.Schedule(ctx, &pb.ScheduleRequest{ClusterId: clusterID})
		if err != nil {
			lastErr = fmt.Errorf("scheduler failed: %w", err)
			slog.Warn("scheduler failed to select node", "database_id", dbID, "attempt", attempts, "error", err)
			time.Sleep(100 * time.Millisecond) // brief wait before retry
			continue
		}

		nodeID := schedRes.GetNodeId()
		if nodeID == "" {
			lastErr = fmt.Errorf("no healthy nodes available in cluster")
			slog.Warn("scheduler returned empty node_id", "database_id", dbID, "attempt", attempts)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		slog.Info("scheduler selected node for database placement", "database_id", dbID, "node_id", nodeID, "score", schedRes.GetScore())

		// 2. Fetch node details from Metadata Service to resolve hostname
		nodesRes, err := o.metadataClient.GetNodes(ctx, &pb.GetNodesRequest{ClusterId: clusterID})
		if err != nil {
			lastErr = fmt.Errorf("failed to fetch nodes details: %w", err)
			slog.Warn("failed to fetch nodes list", "database_id", dbID, "error", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		var targetNodeHostname string
		for _, n := range nodesRes.GetNodes() {
			if n.GetId() == nodeID {
				targetNodeHostname = n.GetHostname()
				break
			}
		}

		if targetNodeHostname == "" {
			lastErr = fmt.Errorf("node %s details not found in metadata", nodeID)
			slog.Warn("scheduler chosen node not found in metadata list", "node_id", nodeID)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// 3. Write database placement choice and current attempts in Metadata
		_, err = o.metadataClient.UpdateDatabaseStatus(ctx, &pb.UpdateDatabaseStatusRequest{
			DatabaseId: dbID,
			Status:     "provisioning",
			NodeId:     nodeID,
			Attempts:   int32(attempts),
		})
		if err != nil {
			slog.Warn("failed to update provisioning metadata with node choice", "database_id", dbID, "node_id", nodeID, "error", err)
		}

		// 4. Dial Node Agent and invoke CreateDatabase
		var agentAddr string
		// Resolves by hostname in bridge network or localhost in local test
		if targetNodeHostname == "worker-local" || targetNodeHostname == "test-worker-node" || targetNodeHostname == "localhost" {
			agentAddr = "localhost:50053"
		} else {
			agentAddr = fmt.Sprintf("%s:50053", targetNodeHostname)
		}

		slog.Info("dialing NodeAgent", "database_id", dbID, "address", agentAddr)

		provisionCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		agentConn, dialErr := grpc.DialContext(provisionCtx, agentAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if dialErr != nil {
			cancel()
			lastErr = fmt.Errorf("failed to dial NodeAgent at %s: %w", agentAddr, dialErr)
			slog.Warn("failed to connect to NodeAgent", "database_id", dbID, "address", agentAddr, "error", dialErr)
			continue
		}

		agentClient := pbAgent.NewNodeAgentClient(agentConn)
		createRes, createErr := agentClient.CreateDatabase(provisionCtx, &pbAgent.CreateDatabaseRequest{
			Name:       name,
			DatabaseId: dbID,
		})
		cancel()
		agentConn.Close()

		if createErr != nil {
			lastErr = fmt.Errorf("NodeAgent CreateDatabase RPC failed: %w", createErr)
			slog.Warn("NodeAgent CreateDatabase RPC failed", "database_id", dbID, "node_id", nodeID, "error", createErr)
			continue
		}

		if !createRes.GetSuccess() {
			lastErr = fmt.Errorf("NodeAgent failed to provision database: %s", createRes.GetError())
			slog.Warn("NodeAgent returned failed creation status", "database_id", dbID, "node_id", nodeID, "error", createRes.GetError())
			continue
		}

		// SUCCESS! Update metadata status to active and store connection endpoint
		endpoint := createRes.GetEndpoint()
		slog.Info("database provisioned successfully on node", "database_id", dbID, "node_id", nodeID, "endpoint", endpoint)

		_, err = o.metadataClient.UpdateDatabaseStatus(ctx, &pb.UpdateDatabaseStatusRequest{
			DatabaseId: dbID,
			Status:     "active",
			NodeId:     nodeID,
			Endpoint:   endpoint,
			Attempts:   int32(attempts),
		})
		if err != nil {
			slog.Error("failed to mark database as active in metadata store", "database_id", dbID, "error", err)
		}
		return
	}

	// ALL ATTEMPTS EXHAUSTED: Mark database as failed
	slog.Error("all provisioning attempts exhausted, marking database as failed", "database_id", dbID, "attempts", attempts, "last_error", lastErr)
	_, err := o.metadataClient.UpdateDatabaseStatus(ctx, &pb.UpdateDatabaseStatusRequest{
		DatabaseId: dbID,
		Status:     "failed",
		Attempts:   int32(attempts),
	})
	if err != nil {
		slog.Error("failed to mark database as failed in metadata store", "database_id", dbID, "error", err)
	}
}
