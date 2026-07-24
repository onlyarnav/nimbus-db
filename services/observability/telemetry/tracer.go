package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/metadata"
)

// InitTracer initializes an OpenTelemetry TracerProvider with OTLP gRPC exporter.
func InitTracer(serviceName string, collectorAddr string) (*sdktrace.TracerProvider, error) {
	if collectorAddr == "" {
		collectorAddr = "localhost:4317"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(collectorAddr),
	)
	if err != nil {
		// Fallback to in-memory/noop tracer if collector unavailable in dev sandbox
		tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
		otel.SetTracerProvider(tp)
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		))
		return tp, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp, nil
}

// ExtractTraceID returns the hex trace ID string from context span, or empty string.
func ExtractTraceID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		return span.SpanContext().TraceID().String()
	}
	return ""
}

// InjectTraceContextToGRPC injects OpenTelemetry trace context into gRPC metadata context.
func InjectTraceContextToGRPC(ctx context.Context) context.Context {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.New(nil)
	}
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	for k, v := range carrier {
		md.Set(k, v)
	}
	return metadata.NewOutgoingContext(ctx, md)
}

// ExtractTraceContextFromGRPC extracts OpenTelemetry trace context from incoming gRPC metadata context.
func ExtractTraceContextFromGRPC(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}
	carrier := propagation.MapCarrier{}
	for k, vs := range md {
		if len(vs) > 0 {
			carrier[k] = vs[0]
		}
	}
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}

// LogStructuredEvent emits a standardized structured JSON log event.
func LogStructuredEvent(ctx context.Context, service string, level string, event string, fields map[string]interface{}) {
	traceID := ExtractTraceID(ctx)

	logObj := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"service":   service,
		"level":     level,
		"event":     event,
		"trace_id":  traceID,
	}

	for k, v := range fields {
		logObj[k] = v
	}

	bytes, err := json.Marshal(logObj)
	if err == nil {
		fmt.Fprintln(os.Stdout, string(bytes))
	} else {
		slog.Error("failed to marshal structured log", "event", event, "error", err)
	}
}

// TracingHTTPMiddleware wraps HTTP handler and creates an OpenTelemetry span.
func TracingHTTPMiddleware(serviceName string, next http.Handler) http.Handler {
	tracer := otel.Tracer(serviceName)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		carrier := propagation.HeaderCarrier(r.Header)
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), carrier)

		ctx, span := tracer.Start(ctx, fmt.Sprintf("HTTP %s %s", r.Method, r.URL.Path))
		defer span.End()

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
