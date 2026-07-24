package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestMetricsInstrumentation(t *testing.T) {
	RequestsTotal.WithLabelValues("test-service", "GET", "200").Inc()
	val := testutil.ToFloat64(RequestsTotal.WithLabelValues("test-service", "GET", "200"))
	if val < 1.0 {
		t.Errorf("expected metric value >= 1.0, got %f", val)
	}

	ErrorsTotal.WithLabelValues("test-service", "POST", "Timeout").Inc()
	errVal := testutil.ToFloat64(ErrorsTotal.WithLabelValues("test-service", "POST", "Timeout"))
	if errVal < 1.0 {
		t.Errorf("expected error metric value >= 1.0, got %f", errVal)
	}
}

func TestStructuredLogEvent(t *testing.T) {
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	otel.SetTracerProvider(tp)
	tracer := otel.Tracer("test")

	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	traceID := ExtractTraceID(ctx)
	if traceID == "" {
		t.Errorf("expected valid trace ID from context span")
	}

	LogStructuredEvent(ctx, "test-service", "INFO", "database_created", map[string]interface{}{
		"database_id": "db-123",
		"node_id":     "node-456",
	})
}

func TestMetricsHandler(t *testing.T) {
	handler := MetricsHandler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 OK from metrics handler, got %d", rec.Code)
	}
}
