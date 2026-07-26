package monitor

import (
	"context"
	"testing"
	"time"

	pb "github.com/onlyarnav/nimbusdb/services/sla-monitor/proto"
)

func TestSLAReportAggregation(t *testing.T) {
	m := NewMonitor()
	ctx := context.Background()

	// Record 999 successful requests and 1 failed request (total 1000 => 99.9% availability)
	for i := 1; i <= 999; i++ {
		_, _ = m.RecordEvent(ctx, &pb.RecordEventRequest{
			EventType: "request_success",
			LatencyMs: int64(i % 50),
		})
	}
	_, _ = m.RecordEvent(ctx, &pb.RecordEventRequest{
		EventType: "request_failure",
		LatencyMs: 150,
	})

	// Record an outage of 15 seconds
	_, _ = m.RecordEvent(ctx, &pb.RecordEventRequest{EventType: "outage_start"})
	time.Sleep(10 * time.Millisecond) // simulated delay
	_, _ = m.RecordEvent(ctx, &pb.RecordEventRequest{EventType: "outage_resolved"})

	res, err := m.GetSLAReport(ctx, &pb.GetSLAReportRequest{WindowMinutes: 60})
	if err != nil {
		t.Fatalf("GetSLAReport failed: %v", err)
	}

	if res.TotalRequests != 1000 {
		t.Errorf("expected total requests 1000, got %d", res.TotalRequests)
	}
	if res.FailedRequests != 1 {
		t.Errorf("expected failed requests 1, got %d", res.FailedRequests)
	}
	if res.AvailabilityPct < 99.89 || res.AvailabilityPct > 99.91 {
		t.Errorf("expected availability ~99.9%%, got %.4f%%", res.AvailabilityPct)
	}
	if !res.SloMet {
		t.Errorf("expected SLO met true, got false")
	}

	t.Logf("SLA Report: Availability=%.2f%%, P95=%dms, P99=%dms, Reqs=%d, Failed=%d, MTTR=%ds, SLO_Met=%v",
		res.AvailabilityPct, res.P95LatencyMs, res.P99LatencyMs, res.TotalRequests, res.FailedRequests, res.MttrSeconds, res.SloMet)
}
