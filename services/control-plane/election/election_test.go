package election

import (
	"testing"

	"github.com/onlyarnav/nimbusdb/services/metadata-service/region"
)

func TestElectNewLeaderRegion_HighestLSN(t *testing.T) {
	candidates := []FollowerCandidate{
		{Region: "us-east", NodeID: "node-us-east-1", AppliedLSN: 100},
		{Region: "us-west", NodeID: "node-us-west-1", AppliedLSN: 150},
		{Region: "europe", NodeID: "node-europe-1", AppliedLSN: 120},
	}

	health := map[string]region.RegionStatus{
		"us-east": region.StatusHealthy,
		"us-west": region.StatusHealthy,
		"europe":  region.StatusHealthy,
	}

	res, err := ElectNewLeaderRegion(candidates, health)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ElectedLeaderRegion != "us-west" {
		t.Errorf("expected us-west (highest LSN 150), got %s", res.ElectedLeaderRegion)
	}
	if res.ElectedNodeID != "node-us-west-1" {
		t.Errorf("expected node-us-west-1, got %s", res.ElectedNodeID)
	}
}

func TestElectNewLeaderRegion_PriorityTieBreaker(t *testing.T) {
	// Equal LSNs (200), priority should favor us-east over us-west and europe
	candidates := []FollowerCandidate{
		{Region: "us-west", NodeID: "node-us-west-1", AppliedLSN: 200},
		{Region: "us-east", NodeID: "node-us-east-1", AppliedLSN: 200},
		{Region: "europe", NodeID: "node-europe-1", AppliedLSN: 200},
	}

	health := map[string]region.RegionStatus{
		"us-east": region.StatusHealthy,
		"us-west": region.StatusHealthy,
		"europe":  region.StatusHealthy,
	}

	res, err := ElectNewLeaderRegion(candidates, health)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ElectedLeaderRegion != "us-east" {
		t.Errorf("expected us-east via priority tie breaker, got %s", res.ElectedLeaderRegion)
	}
}

func TestElectNewLeaderRegion_FilterDownRegions(t *testing.T) {
	candidates := []FollowerCandidate{
		{Region: "us-east", NodeID: "node-us-east-1", AppliedLSN: 300}, // Highest LSN, but region is DOWN
		{Region: "us-west", NodeID: "node-us-west-1", AppliedLSN: 250}, // Healthy candidate
	}

	health := map[string]region.RegionStatus{
		"us-east": region.StatusDown,
		"us-west": region.StatusHealthy,
	}

	res, err := ElectNewLeaderRegion(candidates, health)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ElectedLeaderRegion != "us-west" {
		t.Errorf("expected us-west since us-east is down, got %s", res.ElectedLeaderRegion)
	}
}

func TestElectNewLeaderRegion_NoEligible(t *testing.T) {
	candidates := []FollowerCandidate{
		{Region: "us-east", NodeID: "node-us-east-1", AppliedLSN: 300},
	}

	health := map[string]region.RegionStatus{
		"us-east": region.StatusDown,
	}

	_, err := ElectNewLeaderRegion(candidates, health)
	if err == nil {
		t.Fatalf("expected error when all candidates down, got nil")
	}
}
