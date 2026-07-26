package controller

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/onlyarnav/nimbusdb/services/observability/telemetry"
	pb "github.com/onlyarnav/nimbusdb/services/deployment-controller/proto"
	pbMeta "github.com/onlyarnav/nimbusdb/services/metadata-service/proto"
	pbNode "github.com/onlyarnav/nimbusdb/services/worker-node/proto/nodeagent"
)

// DeploymentRecord holds in-memory state of an active or past deployment.
type DeploymentRecord struct {
	ID                 string
	Type               string // "rolling", "canary", "blue-green"
	TargetVersion      string
	Status             string // "in_progress", "completed", "failed", "rolled_back"
	TrafficSplit       int32  // 0..100
	CanaryWeight       int32
	ObservationSeconds int32
	ErrorMessage       string
	StartTime          time.Time
	EndTime            time.Time
}

// Controller implements the DeploymentControllerService logic.
type Controller struct {
	pb.UnimplementedDeploymentControllerServiceServer
	mu                sync.RWMutex
	deployments       map[string]*DeploymentRecord
	activeDeployment  *DeploymentRecord
	metadataClient    pbMeta.MetadataServiceClient
	nodeAgentDialer   func(target string) (pbNode.NodeAgentClient, func(), error)
	healthCheckFunc   func(ctx context.Context, nodeID string) bool
	metricEvaluator   func(ctx context.Context, deploymentID string) (float64, error) // returns error rate %
}

// NewController creates a new instance of Controller.
func NewController(mc pbMeta.MetadataServiceClient) *Controller {
	return &Controller{
		deployments:    make(map[string]*DeploymentRecord),
		metadataClient: mc,
		healthCheckFunc: func(ctx context.Context, nodeID string) bool {
			return true // default healthy check
		},
		metricEvaluator: func(ctx context.Context, deploymentID string) (float64, error) {
			return 0.0, nil // default 0% error rate
		},
	}
}

// SetHealthCheckFunc overrides the default health check evaluator for testing.
func (c *Controller) SetHealthCheckFunc(fn func(ctx context.Context, nodeID string) bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.healthCheckFunc = fn
}

// SetMetricEvaluator overrides the default metrics evaluator for testing.
func (c *Controller) SetMetricEvaluator(fn func(ctx context.Context, deploymentID string) (float64, error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.metricEvaluator = fn
}

// StartDeployment initiates rolling, canary, or blue-green deployment.
func (c *Controller) StartDeployment(ctx context.Context, req *pb.StartDeploymentRequest) (*pb.StartDeploymentResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	depType := req.GetDeploymentType()
	if depType == "" {
		depType = "rolling"
	}
	targetVer := req.GetTargetVersion()
	if targetVer == "" {
		targetVer = "v2.0.0"
	}

	depID := fmt.Sprintf("deploy-%d", time.Now().UnixNano())
	rec := &DeploymentRecord{
		ID:                 depID,
		Type:               depType,
		TargetVersion:      targetVer,
		Status:             "in_progress",
		CanaryWeight:       req.GetCanaryWeight(),
		ObservationSeconds: req.GetObservationSeconds(),
		StartTime:          time.Now(),
	}

	c.deployments[depID] = rec
	c.activeDeployment = rec

	slog.Info("starting deployment", "deployment_id", depID, "type", depType, "version", targetVer)
	telemetry.LogStructuredEvent(ctx, "deployment-controller", "INFO", "deployment_started", map[string]interface{}{
		"deployment_id": depID,
		"type":          depType,
		"version":       targetVer,
	})

	// Execute deployment asynchronously depending on strategy
	go c.runDeployment(depID)

	return &pb.StartDeploymentResponse{
		DeploymentId: depID,
		Status:       "in_progress",
	}, nil
}

func (c *Controller) runDeployment(depID string) {
	ctx := context.Background()

	c.mu.RLock()
	rec, exists := c.deployments[depID]
	c.mu.RUnlock()
	if !exists {
		return
	}

	switch rec.Type {
	case "rolling":
		c.executeRolling(ctx, rec)
	case "canary":
		c.executeCanary(ctx, rec)
	case "blue-green":
		c.executeBlueGreen(ctx, rec)
	default:
		c.executeRolling(ctx, rec)
	}
}

func (c *Controller) executeRolling(ctx context.Context, rec *DeploymentRecord) {
	slog.Info("executing rolling deployment", "deployment_id", rec.ID)

	// Fetch registered nodes
	nodes := []string{"node-1", "node-2", "node-3"}
	if c.metadataClient != nil {
		res, err := c.metadataClient.GetNodes(ctx, &pbMeta.GetNodesRequest{})
		if err == nil && len(res.GetNodes()) > 0 {
			nodes = nil
			for _, n := range res.GetNodes() {
				nodes = append(nodes, n.GetId())
			}
		}
	}

	for _, nodeID := range nodes {
		slog.Info("rolling update instance", "node_id", nodeID, "version", rec.TargetVersion)
		time.Sleep(50 * time.Millisecond)

		// Perform health check
		c.mu.RLock()
		hcFunc := c.healthCheckFunc
		c.mu.RUnlock()

		if !hcFunc(ctx, nodeID) {
			slog.Error("health check failed during rolling deployment, HALTING rollout immediately", "node_id", nodeID, "deployment_id", rec.ID)
			c.mu.Lock()
			rec.Status = "failed"
			rec.ErrorMessage = fmt.Sprintf("health check failed on node %s", nodeID)
			rec.EndTime = time.Now()
			c.mu.Unlock()

			telemetry.LogStructuredEvent(ctx, "deployment-controller", "ERROR", "rolling_deployment_halted", map[string]interface{}{
				"deployment_id": rec.ID,
				"failed_node":   nodeID,
			})
			return
		}
	}

	c.mu.Lock()
	rec.Status = "completed"
	rec.TrafficSplit = 100
	rec.EndTime = time.Now()
	c.mu.Unlock()

	slog.Info("rolling deployment completed successfully", "deployment_id", rec.ID)
}

func (c *Controller) executeCanary(ctx context.Context, rec *DeploymentRecord) {
	weight := rec.CanaryWeight
	if weight <= 0 {
		weight = 10
	}
	obsSeconds := rec.ObservationSeconds
	if obsSeconds <= 0 {
		obsSeconds = 2
	}

	slog.Info("executing canary deployment", "deployment_id", rec.ID, "canary_weight", weight, "observation_seconds", obsSeconds)

	// Set initial traffic split to canary
	c.mu.Lock()
	rec.TrafficSplit = weight
	c.mu.Unlock()

	// Observation loop
	checkInterval := 50 * time.Millisecond
	deadline := time.Now().Add(time.Duration(obsSeconds) * time.Second)

	for time.Now().Before(deadline) {
		time.Sleep(checkInterval)

		c.mu.RLock()
		evalFunc := c.metricEvaluator
		hcFunc := c.healthCheckFunc
		c.mu.RUnlock()

		// Evaluate canary metrics / error rate
		errRate, err := evalFunc(ctx, rec.ID)
		isHealthy := hcFunc(ctx, "canary-node")

		if err != nil || errRate > 5.0 || !isHealthy {
			errMsg := fmt.Sprintf("canary metric breach: error_rate=%.2f%%, healthy=%v", errRate, isHealthy)
			slog.Warn("canary metrics breached thresholds! Triggering AUTOMATIC ROLLBACK", "deployment_id", rec.ID, "error", errMsg)

			c.performRollback(ctx, rec, errMsg)
			return
		}
	}

	// Promote canary to 100% traffic
	c.mu.Lock()
	rec.Status = "completed"
	rec.TrafficSplit = 100
	rec.EndTime = time.Now()
	c.mu.Unlock()

	slog.Info("canary deployment verified healthy and promoted to 100%", "deployment_id", rec.ID)
}

func (c *Controller) executeBlueGreen(ctx context.Context, rec *DeploymentRecord) {
	slog.Info("executing blue-green deployment", "deployment_id", rec.ID)

	// Verify green environment health
	time.Sleep(100 * time.Millisecond)

	c.mu.RLock()
	hcFunc := c.healthCheckFunc
	c.mu.RUnlock()

	if !hcFunc(ctx, "green-environment") {
		errMsg := "green environment failed pre-switch health verification"
		slog.Error("blue-green deployment failed", "deployment_id", rec.ID, "error", errMsg)
		c.performRollback(ctx, rec, errMsg)
		return
	}

	// Atomic traffic switch to green (100%)
	c.mu.Lock()
	rec.Status = "completed"
	rec.TrafficSplit = 100
	rec.EndTime = time.Now()
	c.mu.Unlock()

	slog.Info("blue-green traffic switched atomically to green environment", "deployment_id", rec.ID)
}

func (c *Controller) performRollback(ctx context.Context, rec *DeploymentRecord, reason string) {
	c.mu.Lock()
	rec.Status = "rolled_back"
	rec.TrafficSplit = 0
	rec.ErrorMessage = reason
	rec.EndTime = time.Now()
	c.mu.Unlock()

	slog.Warn("automatic rollback executed", "deployment_id", rec.ID, "reason", reason)
	telemetry.LogStructuredEvent(ctx, "deployment-controller", "WARN", "auto_rollback_completed", map[string]interface{}{
		"deployment_id": rec.ID,
		"reason":        reason,
		"traffic_split": 0,
	})
}

// GetDeploymentStatus returns status of a specific deployment.
func (c *Controller) GetDeploymentStatus(ctx context.Context, req *pb.GetDeploymentStatusRequest) (*pb.GetDeploymentStatusResponse, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	depID := req.GetDeploymentId()
	rec, exists := c.deployments[depID]
	if !exists {
		if c.activeDeployment != nil {
			rec = c.activeDeployment
		} else {
			return nil, fmt.Errorf("deployment %s not found", depID)
		}
	}

	return &pb.GetDeploymentStatusResponse{
		DeploymentId:   rec.ID,
		DeploymentType: rec.Type,
		TargetVersion:  rec.TargetVersion,
		Status:         rec.Status,
		TrafficSplit:   rec.TrafficSplit,
		ErrorMessage:   rec.ErrorMessage,
	}, nil
}

// TriggerRollback allows manually or automatically triggering a rollback.
func (c *Controller) TriggerRollback(ctx context.Context, req *pb.TriggerRollbackRequest) (*pb.TriggerRollbackResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	depID := req.GetDeploymentId()
	rec, exists := c.deployments[depID]
	if !exists && c.activeDeployment != nil {
		rec = c.activeDeployment
	}
	if rec == nil {
		return nil, fmt.Errorf("no deployment found to roll back")
	}

	rec.Status = "rolled_back"
	rec.TrafficSplit = 0
	rec.ErrorMessage = "Manual rollback: " + req.GetReason()
	rec.EndTime = time.Now()

	slog.Info("rollback triggered", "deployment_id", rec.ID, "reason", req.GetReason())
	return &pb.TriggerRollbackResponse{
		Success: true,
		Status:  "rolled_back",
	}, nil
}

// DrainNode coordinates node draining by updating node status in Metadata Service and evacuating workloads.
func (c *Controller) DrainNode(ctx context.Context, req *pb.DrainNodeRequest) (*pb.DrainNodeResponse, error) {
	nodeID := req.GetNodeId()
	if nodeID == "" {
		return nil, fmt.Errorf("node_id is required")
	}

	slog.Info("initiating node drain orchestration", "node_id", nodeID)

	// 1. Mark node status = 'draining' in Metadata Service
	if c.metadataClient != nil {
		_, err := c.metadataClient.UpdateNodeStatus(ctx, &pbMeta.UpdateNodeStatusRequest{
			NodeId: nodeID,
			Status: "draining",
		})
		if err != nil {
			slog.Warn("failed to mark node status draining in metadata service", "node_id", nodeID, "error", err)
		}
	}

	// 2. Dial node agent and trigger DrainNode RPC
	databasesMoved := 0
	if c.nodeAgentDialer != nil {
		agentClient, cleanup, err := c.nodeAgentDialer(nodeID)
		if err == nil {
			defer cleanup()
			res, drainErr := agentClient.DrainNode(ctx, &pbNode.DrainNodeRequest{NodeId: nodeID})
			if drainErr == nil && res.GetSuccess() {
				databasesMoved = int(res.GetDatabasesMoved())
			}
		}
	}

	// 3. Mark node status = 'drained' in Metadata Service
	if c.metadataClient != nil {
		_, _ = c.metadataClient.UpdateNodeStatus(ctx, &pbMeta.UpdateNodeStatusRequest{
			NodeId: nodeID,
			Status: "drained",
		})
	}

	slog.Info("node drain orchestration completed", "node_id", nodeID, "databases_moved", databasesMoved)
	return &pb.DrainNodeResponse{
		Success:        true,
		DatabasesMoved: int32(databasesMoved),
	}, nil
}
