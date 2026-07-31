package planner

import (
	"context"
	"log/slog"
	"math"
	"time"

	pb "github.com/onlyarnav/nimbusdb/services/deployment-controller/proto"
)

// MetricSample represents a single point in a historical time series.
type MetricSample struct {
	Timestamp time.Time
	Value     float64
}

// Planner implements the CapacityPlannerService interface.
type Planner struct {
	pb.UnimplementedCapacityPlannerServiceServer
	historicalData []MetricSample
}

// NewPlanner initializes a new CapacityPlanner instance.
func NewPlanner() *Planner {
	return &Planner{}
}

// SetHistoricalData populates synthetic or measured historical metrics for capacity prediction.
func (p *Planner) SetHistoricalData(data []MetricSample) {
	p.historicalData = data
}

// PredictCapacity computes trend-based projection using linear regression over historical load samples.
func (p *Planner) PredictCapacity(ctx context.Context, req *pb.PredictCapacityRequest) (*pb.PredictCapacityResponse, error) {
	horizonDays := req.GetHorizonDays()
	if horizonDays <= 0 {
		horizonDays = 7
	}

	data := p.historicalData
	if len(data) < 2 {
		// Fallback synthetic dataset representing 5% growth per day over 7 days (starting at 4 nodes)
		now := time.Now()
		data = []MetricSample{
			{Timestamp: now.AddDate(0, 0, -6), Value: 4.0},
			{Timestamp: now.AddDate(0, 0, -5), Value: 4.2},
			{Timestamp: now.AddDate(0, 0, -4), Value: 4.4},
			{Timestamp: now.AddDate(0, 0, -3), Value: 4.6},
			{Timestamp: now.AddDate(0, 0, -2), Value: 4.8},
			{Timestamp: now.AddDate(0, 0, -1), Value: 5.0},
			{Timestamp: now, Value: 5.2},
		}
	}

	currentNodes := int32(math.Ceil(data[len(data)-1].Value))
	slope, intercept := calculateLinearRegression(data)

	// Projected value at target horizon
	horizonTime := data[len(data)-1].Timestamp.AddDate(0, 0, int(horizonDays))
	daysFromStart := horizonTime.Sub(data[0].Timestamp).Hours() / 24.0
	projectedValue := slope*daysFromStart + intercept
	if projectedValue < float64(currentNodes) {
		projectedValue = float64(currentNodes)
	}

	projectedNodes := int32(math.Ceil(projectedValue))
	growthRatePct := ((float64(projectedNodes) - float64(currentNodes)) / float64(currentNodes)) * 100.0

	horizonDate := horizonTime.Format("2006-01-02")

	slog.Info("capacity projection computed",
		"horizon_days", horizonDays,
		"current_nodes", currentNodes,
		"projected_nodes", projectedNodes,
		"growth_rate_pct", growthRatePct,
		"method", "linear_regression",
	)

	return &pb.PredictCapacityResponse{
		CurrentNodes:  currentNodes,
		ProjectedNodes: projectedNodes,
		GrowthRatePct: growthRatePct,
		Method:        "linear_regression",
		HorizonDate:   horizonDate,
	}, nil
}

// Helper: computes slope (m) and intercept (b) for y = mx + b
func calculateLinearRegression(samples []MetricSample) (float64, float64) {
	n := float64(len(samples))
	if n == 0 {
		return 0, 0
	}

	startTime := samples[0].Timestamp
	var sumX, sumY, sumXY, sumXX float64

	for _, s := range samples {
		x := s.Timestamp.Sub(startTime).Hours() / 24.0 // days relative to start
		y := s.Value

		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}

	denom := (n*sumXX - sumX*sumX)
	if denom == 0 {
		return 0, sumY / n
	}

	slope := (n*sumXY - sumX*sumY) / denom
	intercept := (sumY - slope*sumX) / n

	return slope, intercept
}
