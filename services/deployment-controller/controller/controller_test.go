package controller

import (
	"context"
	"testing"
	"time"

	pb "github.com/onlyarnav/nimbusdb/services/deployment-controller/proto"
)

func TestRollingDeploymentSuccess(t *testing.T) {
	c := NewController(nil)
	ctx := context.Background()

	res, err := c.StartDeployment(ctx, &pb.StartDeploymentRequest{
		DeploymentType: "rolling",
		TargetVersion:  "v2.0.0",
	})
	if err != nil {
		t.Fatalf("StartDeployment failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	status, err := c.GetDeploymentStatus(ctx, &pb.GetDeploymentStatusRequest{DeploymentId: res.DeploymentId})
	if err != nil {
		t.Fatalf("GetDeploymentStatus failed: %v", err)
	}

	if status.Status != "completed" {
		t.Errorf("expected status 'completed', got %q", status.Status)
	}
	if status.TrafficSplit != 100 {
		t.Errorf("expected traffic split 100, got %d", status.TrafficSplit)
	}
}

func TestRollingDeploymentHaltOnHealthCheckFailure(t *testing.T) {
	c := NewController(nil)
	ctx := context.Background()

	// Fail health check on node-2
	c.SetHealthCheckFunc(func(ctx context.Context, nodeID string) bool {
		return nodeID != "node-2"
	})

	res, err := c.StartDeployment(ctx, &pb.StartDeploymentRequest{
		DeploymentType: "rolling",
		TargetVersion:  "v2.0.0",
	})
	if err != nil {
		t.Fatalf("StartDeployment failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	status, err := c.GetDeploymentStatus(ctx, &pb.GetDeploymentStatusRequest{DeploymentId: res.DeploymentId})
	if err != nil {
		t.Fatalf("GetDeploymentStatus failed: %v", err)
	}

	if status.Status != "failed" {
		t.Errorf("expected status 'failed', got %q", status.Status)
	}
}

func TestCanaryAutoRollbackOnMetricBreach(t *testing.T) {
	c := NewController(nil)
	ctx := context.Background()

	// Simulate metric breach (12% error rate)
	c.SetMetricEvaluator(func(ctx context.Context, deploymentID string) (float64, error) {
		return 12.5, nil
	})

	start := time.Now()
	res, err := c.StartDeployment(ctx, &pb.StartDeploymentRequest{
		DeploymentType:     "canary",
		TargetVersion:      "v2.0.0-broken",
		CanaryWeight:       10,
		ObservationSeconds: 2,
	})
	if err != nil {
		t.Fatalf("StartDeployment failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	status, err := c.GetDeploymentStatus(ctx, &pb.GetDeploymentStatusRequest{DeploymentId: res.DeploymentId})
	if err != nil {
		t.Fatalf("GetDeploymentStatus failed: %v", err)
	}

	rollbackTime := time.Since(start)

	if status.Status != "rolled_back" {
		t.Errorf("expected status 'rolled_back', got %q", status.Status)
	}
	if status.TrafficSplit != 0 {
		t.Errorf("expected traffic split 0 post-rollback, got %d", status.TrafficSplit)
	}

	t.Logf("Canary detection-to-rollback time: %v", rollbackTime)
}

func TestBlueGreenDeploymentSuccess(t *testing.T) {
	c := NewController(nil)
	ctx := context.Background()

	res, err := c.StartDeployment(ctx, &pb.StartDeploymentRequest{
		DeploymentType: "blue-green",
		TargetVersion:  "v2.0.0",
	})
	if err != nil {
		t.Fatalf("StartDeployment failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	status, err := c.GetDeploymentStatus(ctx, &pb.GetDeploymentStatusRequest{DeploymentId: res.DeploymentId})
	if err != nil {
		t.Fatalf("GetDeploymentStatus failed: %v", err)
	}

	if status.Status != "completed" {
		t.Errorf("expected status 'completed', got %q", status.Status)
	}
	if status.TrafficSplit != 100 {
		t.Errorf("expected traffic split 100, got %d", status.TrafficSplit)
	}
}

func TestNodeDrainOrchestration(t *testing.T) {
	c := NewController(nil)
	ctx := context.Background()

	res, err := c.DrainNode(ctx, &pb.DrainNodeRequest{NodeId: "worker-node-1"})
	if err != nil {
		t.Fatalf("DrainNode failed: %v", err)
	}

	if !res.Success {
		t.Errorf("expected DrainNode success true, got false")
	}
}
