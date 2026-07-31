package monitor

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	pb "github.com/onlyarnav/nimbusdb/services/deployment-controller/proto"
)

// RequestRecord tracks latency and success status of a single request.
type RequestRecord struct {
	Timestamp time.Time
	Success   bool
	LatencyMs int64
}

// OutageRecord tracks failure start and resolution time.
type OutageRecord struct {
	StartTime    time.Time
	ResolvedTime time.Time
}

// Monitor implements the SLAMonitorService interface.
type Monitor struct {
	pb.UnimplementedSLAMonitorServiceServer
	mu             sync.RWMutex
	records        []RequestRecord
	outages        []OutageRecord
	activeOutageAt *time.Time
}

// NewMonitor initializes a new SLAMonitor instance.
func NewMonitor() *Monitor {
	return &Monitor{}
}

// RecordEvent records a request or outage lifecycle event.
func (m *Monitor) RecordEvent(ctx context.Context, req *pb.RecordEventRequest) (*pb.RecordEventResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	eventType := req.GetEventType()
	now := time.Now()

	switch eventType {
	case "request_success":
		m.records = append(m.records, RequestRecord{
			Timestamp: now,
			Success:   true,
			LatencyMs: req.GetLatencyMs(),
		})
	case "request_failure":
		m.records = append(m.records, RequestRecord{
			Timestamp: now,
			Success:   false,
			LatencyMs: req.GetLatencyMs(),
		})
	case "outage_start":
		if m.activeOutageAt == nil {
			m.activeOutageAt = &now
			slog.Warn("SLA monitor recorded outage start", "timestamp", now)
		}
	case "outage_resolved":
		if m.activeOutageAt != nil {
			outage := OutageRecord{
				StartTime:    *m.activeOutageAt,
				ResolvedTime: now,
			}
			m.outages = append(m.outages, outage)
			m.activeOutageAt = nil
			slog.Info("SLA monitor recorded outage resolution", "duration_seconds", now.Sub(outage.StartTime).Seconds())
		}
	}

	return &pb.RecordEventResponse{Success: true}, nil
}

// GetSLAReport computes availability %, percentiles, and MTTR over requested window.
func (m *Monitor) GetSLAReport(ctx context.Context, req *pb.GetSLAReportRequest) (*pb.GetSLAReportResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	windowMins := req.GetWindowMinutes()
	if windowMins <= 0 {
		windowMins = 60
	}

	cutoff := time.Now().Add(-time.Duration(windowMins) * time.Minute)

	var totalReqs, failedReqs int64
	var latencies []int64

	for _, r := range m.records {
		if r.Timestamp.After(cutoff) {
			totalReqs++
			if !r.Success {
				failedReqs++
			}
			if r.LatencyMs > 0 {
				latencies = append(latencies, r.LatencyMs)
			}
		}
	}

	var availPct float64 = 100.0
	if totalReqs > 0 {
		availPct = (float64(totalReqs-failedReqs) / float64(totalReqs)) * 100.0
	}

	var p95, p99 int64
	if len(latencies) > 0 {
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		p95Idx := int(float64(len(latencies)) * 0.95)
		p99Idx := int(float64(len(latencies)) * 0.99)
		if p95Idx >= len(latencies) {
			p95Idx = len(latencies) - 1
		}
		if p99Idx >= len(latencies) {
			p99Idx = len(latencies) - 1
		}
		p95 = latencies[p95Idx]
		p99 = latencies[p99Idx]
	}

	// Calculate MTTR in seconds
	var totalRecoverySecs float64
	var outageCount int64
	for _, o := range m.outages {
		if o.ResolvedTime.After(cutoff) {
			totalRecoverySecs += o.ResolvedTime.Sub(o.StartTime).Seconds()
			outageCount++
		}
	}

	var mttrSecs int64
	if outageCount > 0 {
		mttrSecs = int64(totalRecoverySecs / float64(outageCount))
	}

	sloMet := availPct >= 99.9

	slog.Info("SLA report generated",
		"availability_pct", availPct,
		"p95_ms", p95,
		"p99_ms", p99,
		"total_requests", totalReqs,
		"failed_requests", failedReqs,
		"mttr_seconds", mttrSecs,
		"slo_met", sloMet,
	)

	return &pb.GetSLAReportResponse{
		AvailabilityPct: availPct,
		P95LatencyMs:    p95,
		P99LatencyMs:    p99,
		TotalRequests:   totalReqs,
		FailedRequests:  failedReqs,
		MttrSeconds:     mttrSecs,
		SloMet:          sloMet,
	}, nil
}
