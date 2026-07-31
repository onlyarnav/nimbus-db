package planner

import (
	"context"
	"testing"
	"time"

	pb "github.com/onlyarnav/nimbusdb/services/deployment-controller/proto"
)

func TestPredictCapacityLinearTrend(t *testing.T) {
	p := NewPlanner()
	ctx := context.Background()

	now := time.Now()
	// Synthetic dataset with known linear growth trend: 10 nodes growing by +1 node per week
	data := []MetricSample{
		{Timestamp: now.AddDate(0, 0, -21), Value: 10.0},
		{Timestamp: now.AddDate(0, 0, -14), Value: 11.0},
		{Timestamp: now.AddDate(0, 0, -7), Value: 12.0},
		{Timestamp: now, Value: 13.0},
	}
	p.SetHistoricalData(data)

	// Predict capacity 14 days into the future (expected: +2 nodes -> 15 nodes)
	res, err := p.PredictCapacity(ctx, &pb.PredictCapacityRequest{
		HorizonDays: 14,
		Metric:      "nodes",
	})
	if err != nil {
		t.Fatalf("PredictCapacity failed: %v", err)
	}

	if res.CurrentNodes != 13 {
		t.Errorf("expected current nodes 13, got %d", res.CurrentNodes)
	}
	if res.ProjectedNodes < 15 {
		t.Errorf("expected projected nodes >= 15, got %d", res.ProjectedNodes)
	}
	if res.Method != "linear_regression" {
		t.Errorf("expected method linear_regression, got %s", res.Method)
	}

	t.Logf("Capacity projection output: Current=%d, Projected=%d, Growth=%.2f%%, HorizonDate=%s",
		res.CurrentNodes, res.ProjectedNodes, res.GrowthRatePct, res.HorizonDate)
}
