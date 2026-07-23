package placement

import (
	"testing"

	pb "github.com/onlyarnav/nimbusdb/services/scheduler/proto"
)

func TestScheduleNodeWithRegionFallback_PreferredHealthy(t *testing.T) {
	nodes := []*pb.NodeInfo{
		{Id: "node-india-1", Hostname: "worker-india-01", Status: "healthy", CpuPct: 10, MemoryPct: 20, DiskPct: 30},
		{Id: "node-us-east-1", Hostname: "worker-us-east-01", Status: "healthy", CpuPct: 50, MemoryPct: 50, DiskPct: 50},
	}

	res, err := ScheduleNodeWithRegionFallback(nodes, "india")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ServedRegion != "india" {
		t.Errorf("expected served region india, got %s", res.ServedRegion)
	}
	if res.Node.NodeID != "node-india-1" {
		t.Errorf("expected node-india-1, got %s", res.Node.NodeID)
	}
	if res.FallbackUsed {
		t.Errorf("expected fallbackUsed = false")
	}
}

func TestScheduleNodeWithRegionFallback_PreferredDownFallbackToNextNearest(t *testing.T) {
	// us-east is preferred, but all us-east nodes are dead
	nodes := []*pb.NodeInfo{
		{Id: "node-us-east-1", Hostname: "worker-us-east-01", Status: "dead", CpuPct: 0, MemoryPct: 0, DiskPct: 0},
		{Id: "node-us-west-1", Hostname: "worker-us-west-01", Status: "healthy", CpuPct: 15, MemoryPct: 25, DiskPct: 20},
		{Id: "node-india-1", Hostname: "worker-india-01", Status: "healthy", CpuPct: 40, MemoryPct: 40, DiskPct: 40},
	}

	// From us-east, next nearest region is us-west (60ms vs india 180ms)
	res, err := ScheduleNodeWithRegionFallback(nodes, "us-east")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ServedRegion != "us-west" {
		t.Errorf("expected fallback to us-west, got %s", res.ServedRegion)
	}
	if res.Node.NodeID != "node-us-west-1" {
		t.Errorf("expected node-us-west-1, got %s", res.Node.NodeID)
	}
	if !res.FallbackUsed {
		t.Errorf("expected fallbackUsed = true")
	}
}

func TestScheduleNodeWithRegionFallback_AllRegionsDead(t *testing.T) {
	nodes := []*pb.NodeInfo{
		{Id: "node-1", Hostname: "worker-india-01", Status: "dead"},
		{Id: "node-2", Hostname: "worker-us-east-01", Status: "dead"},
	}

	_, err := ScheduleNodeWithRegionFallback(nodes, "india")
	if err == nil {
		t.Fatalf("expected error when all regions dead, got nil")
	}
}
