package router

import (
	"testing"

	"github.com/onlyarnav/nimbusdb/services/metadata-service/region"
)

func TestSelectRegion_PreferredHealthy(t *testing.T) {
	health := map[string]region.RegionStatus{
		"india":   region.StatusHealthy,
		"us-east": region.StatusHealthy,
	}

	res, err := SelectRegion("us-east", health)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ServedRegion != "us-east" {
		t.Errorf("expected served region us-east, got %s", res.ServedRegion)
	}
	if res.FallbackRerouted {
		t.Errorf("expected fallbackRerouted = false")
	}
}

func TestSelectRegion_PreferredDownReroutesToNextNearest(t *testing.T) {
	// us-east is down. Next nearest to us-east is us-west (60ms), then europe (90ms)
	health := map[string]region.RegionStatus{
		"us-east": region.StatusDown,
		"us-west": region.StatusHealthy,
		"europe":  region.StatusHealthy,
		"india":   region.StatusHealthy,
	}

	res, err := SelectRegion("us-east", health)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ServedRegion != "us-west" {
		t.Errorf("expected served region us-west, got %s", res.ServedRegion)
	}
	if !res.FallbackRerouted {
		t.Errorf("expected fallbackRerouted = true")
	}
}

func TestSelectRegion_AllDown(t *testing.T) {
	health := map[string]region.RegionStatus{
		"india":   region.StatusDown,
		"us-east": region.StatusDown,
		"us-west": region.StatusDown,
		"europe":  region.StatusDown,
		"japan":   region.StatusDown,
	}

	_, err := SelectRegion("india", health)
	if err == nil {
		t.Fatalf("expected error when all regions down, got nil")
	}
}
