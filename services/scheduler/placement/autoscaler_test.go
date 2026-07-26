package placement

import (
	"testing"
	"time"

	pb "github.com/onlyarnav/nimbusdb/services/scheduler/proto"
)

func TestAutoScalerScaleOut(t *testing.T) {
	scaler := NewAutoScaler(75.0, 20.0, 5*time.Second)

	nodes := []*pb.NodeInfo{
		{Id: "node-1", Status: "healthy", CpuPct: 80.0, MemoryPct: 70.0},
		{Id: "node-2", Status: "healthy", CpuPct: 85.0, MemoryPct: 75.0},
	}

	decision := scaler.Evaluate(nodes)
	if decision.Action != ActionScaleOut {
		t.Errorf("expected ActionScaleOut, got %s", decision.Action)
	}

	// Immediate re-evaluation should hit cooldown
	cooldownDecision := scaler.Evaluate(nodes)
	if cooldownDecision.Action != ActionNone {
		t.Errorf("expected ActionNone due to cooldown, got %s", cooldownDecision.Action)
	}
}

func TestAutoScalerScaleIn(t *testing.T) {
	scaler := NewAutoScaler(75.0, 20.0, 5*time.Second)

	nodes := []*pb.NodeInfo{
		{Id: "node-1", Status: "healthy", CpuPct: 15.0, MemoryPct: 10.0},
		{Id: "node-2", Status: "healthy", CpuPct: 10.0, MemoryPct: 10.0},
	}

	decision := scaler.Evaluate(nodes)
	if decision.Action != ActionScaleIn {
		t.Errorf("expected ActionScaleIn, got %s", decision.Action)
	}
	if decision.TargetNodeID != "node-2" {
		t.Errorf("expected target node-2 for drain, got %s", decision.TargetNodeID)
	}
}
