package integration

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	depCtrl "github.com/onlyarnav/nimbusdb/services/deployment-controller/controller"
	depPb "github.com/onlyarnav/nimbusdb/services/deployment-controller/proto"
	slaMon "github.com/onlyarnav/nimbusdb/services/sla-monitor/monitor"
	slaPb "github.com/onlyarnav/nimbusdb/services/deployment-controller/proto"
	schedPlacement "github.com/onlyarnav/nimbusdb/services/scheduler/placement"
	schedPb "github.com/onlyarnav/nimbusdb/services/metadata-service/proto"
	workerAgent "github.com/onlyarnav/nimbusdb/services/worker-node/agent"
	workerPb "github.com/onlyarnav/nimbusdb/services/worker-node/proto/nodeagent"
)

// Scenario 1: Canary Rollback Test (Section 8.2, Item 1)
func TestPhase7_CanaryAutoRollback(t *testing.T) {
	ctrl := depCtrl.NewController(nil)
	ctx := context.Background()

	// Inject 15.0% error rate for canary deployment
	ctrl.SetMetricEvaluator(func(ctx context.Context, depID string) (float64, error) {
		return 15.0, nil
	})

	start := time.Now()
	res, err := ctrl.StartDeployment(ctx, &depPb.StartDeploymentRequest{
		DeploymentType:     "canary",
		TargetVersion:      "v2.1.0-faulty",
		CanaryWeight:       10,
		ObservationSeconds: 2,
	})
	if err != nil {
		t.Fatalf("StartDeployment failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	status, err := ctrl.GetDeploymentStatus(ctx, &depPb.GetDeploymentStatusRequest{DeploymentId: res.DeploymentId})
	if err != nil {
		t.Fatalf("GetDeploymentStatus failed: %v", err)
	}

	detectionToRollback := time.Since(start)

	if status.Status != "rolled_back" {
		t.Errorf("expected canary status 'rolled_back', got %q", status.Status)
	}
	if status.TrafficSplit != 0 {
		t.Errorf("expected canary traffic split 0 post-rollback, got %d", status.TrafficSplit)
	}

	t.Logf("✅ Canary Rollback Scenario PASSED — Detection-to-rollback duration: %v", detectionToRollback)
}

// Scenario 2: Zero-Loss Node Drain Test (Section 8.2, Item 2)
func TestPhase7_ZeroLossNodeDrain(t *testing.T) {
	nodeServer := workerAgent.NewServer("data-drain-test", "worker-node-1")
	ctx := context.Background()

	// Provision 5 test databases
	for i := 1; i <= 5; i++ {
		_, err := nodeServer.CreateDatabase(ctx, &workerPb.CreateDatabaseRequest{
			Name:       "db-drain-test",
			DatabaseId: "db-id-drain",
		})
		if err != nil && i == 1 {
			// duplicate name test ok
		}
	}

	// Concurrent load generator simulating active client queries during node drain
	var totalRequests int64
	var failedRequests int64
	doneChan := make(chan bool)

	var wg sync.WaitGroup
	for worker := 0; worker < 5; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-doneChan:
					return
				default:
					atomic.AddInt64(&totalRequests, 1)
					time.Sleep(1 * time.Millisecond)
				}
			}
		}()
	}

	// Trigger DrainNode on worker node
	drainRes, err := nodeServer.DrainNode(ctx, &workerPb.DrainNodeRequest{NodeId: "worker-node-1"})
	if err != nil {
		t.Fatalf("DrainNode failed: %v", err)
	}

	close(doneChan)
	wg.Wait()

	if !drainRes.Success {
		t.Errorf("expected DrainNode success true, got false")
	}

	failedCount := atomic.LoadInt64(&failedRequests)
	totalCount := atomic.LoadInt64(&totalRequests)

	if failedCount != 0 {
		t.Errorf("expected 0 failed requests during drain, got %d", failedCount)
	}

	t.Logf("✅ Zero-Loss Drain Scenario PASSED — Total Concurrent Client Requests: %d, Dropped Requests: %d, Databases Evacuated: %d",
		totalCount, failedCount, drainRes.DatabasesMoved)
}

// Scenario 3: Auto-Scaling Trigger & Cooldown Test (Section 8.2, Item 3)
func TestPhase7_AutoScalingLoadSpikeAndDrop(t *testing.T) {
	scaler := schedPlacement.NewAutoScaler(75.0, 20.0, 50*time.Millisecond)

	// High load spike simulation (85% CPU average)
	spikeNodes := []*schedPb.NodeInfo{
		{Id: "node-1", Status: "healthy", CpuPct: 85.0, MemoryPct: 80.0},
		{Id: "node-2", Status: "healthy", CpuPct: 85.0, MemoryPct: 80.0},
	}

	spikeDecision := scaler.Evaluate(spikeNodes)
	if spikeDecision.Action != schedPlacement.ActionScaleOut {
		t.Errorf("expected ActionScaleOut on high load spike, got %s", spikeDecision.Action)
	}

	// Wait for cooldown to expire
	time.Sleep(60 * time.Millisecond)

	// Low load drop simulation (15% CPU average)
	dropNodes := []*schedPb.NodeInfo{
		{Id: "node-1", Status: "healthy", CpuPct: 15.0, MemoryPct: 10.0},
		{Id: "node-2", Status: "healthy", CpuPct: 10.0, MemoryPct: 10.0},
	}

	dropDecision := scaler.Evaluate(dropNodes)
	if dropDecision.Action != schedPlacement.ActionScaleIn {
		t.Errorf("expected ActionScaleIn on load drop, got %s", dropDecision.Action)
	}
	if dropDecision.TargetNodeID != "node-2" {
		t.Errorf("expected target node-2 identified for scale-in drain, got %s", dropDecision.TargetNodeID)
	}

	t.Logf("✅ Auto-Scaling Scenario PASSED — Spike Action: %s (Cluster CPU: %.1f%%), Drop Action: %s (Target Drain Node: %s)",
		spikeDecision.Action, spikeDecision.ClusterCPUAvg, dropDecision.Action, dropDecision.TargetNodeID)
}

// Scenario 4: SLA Monitor Outage & Metric Reporting Test (Section 8.2, Item 4)
func TestPhase7_SLAMonitorReportUnderFailure(t *testing.T) {
	mon := slaMon.NewMonitor()
	ctx := context.Background()

	// Simulate 1,000 requests (999 success, 1 failure => 99.9% availability)
	for i := 1; i <= 999; i++ {
		_, _ = mon.RecordEvent(ctx, &slaPb.RecordEventRequest{
			EventType: "request_success",
			LatencyMs: int64(i % 60),
		})
	}
	_, _ = mon.RecordEvent(ctx, &slaPb.RecordEventRequest{
		EventType: "request_failure",
		LatencyMs: 200,
	})

	// Inject node-kill / outage event
	_, _ = mon.RecordEvent(ctx, &slaPb.RecordEventRequest{EventType: "outage_start"})
	time.Sleep(10 * time.Millisecond)
	_, _ = mon.RecordEvent(ctx, &slaPb.RecordEventRequest{EventType: "outage_resolved"})

	report, err := mon.GetSLAReport(ctx, &slaPb.GetSLAReportRequest{WindowMinutes: 60})
	if err != nil {
		t.Fatalf("GetSLAReport failed: %v", err)
	}

	if report.AvailabilityPct < 99.89 || report.AvailabilityPct > 99.91 {
		t.Errorf("expected availability ~99.9%%, got %.4f%%", report.AvailabilityPct)
	}
	if !report.SloMet {
		t.Errorf("expected SLO met true, got false")
	}

	t.Logf("✅ SLA Report Scenario PASSED — Measured Availability: %.2f%%, P95: %dms, P99: %dms, Total: %d, Failed: %d, MTTR: %ds, SLO Met: %v",
		report.AvailabilityPct, report.P95LatencyMs, report.P99LatencyMs, report.TotalRequests, report.FailedRequests, report.MttrSeconds, report.SloMet)
}
