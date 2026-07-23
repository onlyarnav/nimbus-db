package region

import (
	"testing"
)

func TestRollupRegionHealth(t *testing.T) {
	tests := []struct {
		name           string
		region         string
		nodes          []NodeState
		expectedStatus RegionStatus
		expectedTotal  int
		expectedHealth int
	}{
		{
			name:   "All nodes healthy",
			region: "india",
			nodes: []NodeState{
				{ID: "n1", Region: "india", Status: "healthy"},
				{ID: "n2", Region: "india", Status: "healthy"},
			},
			expectedStatus: StatusHealthy,
			expectedTotal:  2,
			expectedHealth: 2,
		},
		{
			name:   "Degraded region with one dead node",
			region: "us-east",
			nodes: []NodeState{
				{ID: "n3", Region: "us-east", Status: "healthy"},
				{ID: "n4", Region: "us-east", Status: "dead"},
			},
			expectedStatus: StatusDegraded,
			expectedTotal:  2,
			expectedHealth: 1,
		},
		{
			name:   "Down region with all dead nodes",
			region: "us-west",
			nodes: []NodeState{
				{ID: "n5", Region: "us-west", Status: "dead"},
				{ID: "n6", Region: "us-west", Status: "unhealthy"},
			},
			expectedStatus: StatusDown,
			expectedTotal:  2,
			expectedHealth: 0,
		},
		{
			name:           "Empty region with zero nodes",
			region:         "japan",
			nodes:          []NodeState{},
			expectedStatus: StatusDown,
			expectedTotal:  0,
			expectedHealth: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := RollupRegionHealth(tt.region, tt.nodes)
			if res.Status != tt.expectedStatus {
				t.Errorf("expected status %s, got %s", tt.expectedStatus, res.Status)
			}
			if res.TotalNodes != tt.expectedTotal {
				t.Errorf("expected total nodes %d, got %d", tt.expectedTotal, res.TotalNodes)
			}
			if res.HealthyNodes != tt.expectedHealth {
				t.Errorf("expected healthy nodes %d, got %d", tt.expectedHealth, res.HealthyNodes)
			}
		})
	}
}

func TestGetFallbackRegions(t *testing.T) {
	// From us-east: us-east (0), us-west (60), europe (90), japan (160), india (180)
	fallbacks := GetFallbackRegions("us-east")
	expected := []string{"us-east", "us-west", "europe", "japan", "india"}

	if len(fallbacks) != len(expected) {
		t.Fatalf("expected %d regions, got %d", len(expected), len(fallbacks))
	}

	for i, r := range expected {
		if fallbacks[i] != r {
			t.Errorf("at index %d expected %s, got %s", i, r, fallbacks[i])
		}
	}
}
