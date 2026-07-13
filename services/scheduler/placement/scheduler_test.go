package placement

import (
	"errors"
	"testing"

	pb "github.com/onlyarnav/nimbusdb/services/scheduler/proto"
)

func TestCalculateScore(t *testing.T) {
	score, breakdown := CalculateScore(10, 20, 30) // cpu=10, mem=20, disk=30
	// (100 - 10) * 0.4 = 36
	// (100 - 20) * 0.3 = 24
	// (100 - 30) * 0.3 = 21
	// Total: 81
	expectedScore := float32(81.0)
	if score != expectedScore {
		t.Errorf("expected score %f, got %f", expectedScore, score)
	}
	if breakdown["cpu_factor"] != 36.0 || breakdown["memory_factor"] != 24.0 || breakdown["disk_factor"] != 21.0 {
		t.Errorf("breakdown details mismatch: %+v", breakdown)
	}
}

func TestScheduleNode(t *testing.T) {
	// Scenario 1: Select the least loaded node (highest score) among healthy ones
	nodes1 := []*pb.NodeInfo{
		{Id: "node-a", Status: "healthy", CpuPct: 50, MemoryPct: 50, DiskPct: 50}, // score: 50
		{Id: "node-b", Status: "healthy", CpuPct: 10, MemoryPct: 10, DiskPct: 10}, // score: 90 (best)
		{Id: "node-c", Status: "healthy", CpuPct: 80, MemoryPct: 80, DiskPct: 80}, // score: 20
	}

	best, err := ScheduleNode(nodes1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if best.NodeID != "node-b" {
		t.Errorf("expected node-b, got %q", best.NodeID)
	}

	// Scenario 2: Exclude dead and draining nodes
	nodes2 := []*pb.NodeInfo{
		{Id: "node-a", Status: "dead", CpuPct: 10, MemoryPct: 10, DiskPct: 10},     // excluded
		{Id: "node-b", Status: "draining", CpuPct: 10, MemoryPct: 10, DiskPct: 10}, // excluded
		{Id: "node-c", Status: "healthy", CpuPct: 80, MemoryPct: 80, DiskPct: 80},  // selected (only option)
	}

	best, err = ScheduleNode(nodes2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if best.NodeID != "node-c" {
		t.Errorf("expected node-c, got %q", best.NodeID)
	}

	// Scenario 3: Deprioritize overloaded node over healthy node even if healthy node has lower score
	nodes3 := []*pb.NodeInfo{
		{Id: "node-overloaded", Status: "overloaded", CpuPct: 92, MemoryPct: 10, DiskPct: 10},
		{Id: "node-healthy", Status: "healthy", CpuPct: 70, MemoryPct: 70, DiskPct: 70}, // score = 30
	}

	best, err = ScheduleNode(nodes3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if best.NodeID != "node-healthy" {
		t.Errorf("expected node-healthy, got %q (overloaded was not deprioritized correctly)", best.NodeID)
	}

	// Scenario 4: Pick overloaded node if no other healthy node is available
	nodes4 := []*pb.NodeInfo{
		{Id: "node-overloaded", Status: "overloaded", CpuPct: 92, MemoryPct: 10, DiskPct: 10},
		{Id: "node-dead", Status: "dead", CpuPct: 10, MemoryPct: 10, DiskPct: 10},
	}

	best, err = ScheduleNode(nodes4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if best.NodeID != "node-overloaded" {
		t.Errorf("expected node-overloaded, got %q", best.NodeID)
	}

	// Scenario 5: Error when no candidate nodes exist
	nodes5 := []*pb.NodeInfo{
		{Id: "node-dead", Status: "dead", CpuPct: 10, MemoryPct: 10, DiskPct: 10},
		{Id: "node-draining", Status: "draining", CpuPct: 10, MemoryPct: 10, DiskPct: 10},
	}

	_, err = ScheduleNode(nodes5)
	if !errors.Is(err, ErrNoNodesAvailable) {
		t.Errorf("expected ErrNoNodesAvailable, got %v", err)
	}
}
