# Architectural Decision Record: Observability Tracing Backend

## Context
In Phase 5, NimbusDB implements end-to-end distributed tracing across all microservices (`gateway` → `scheduler` → `control-plane` → `node-agent` → storage engine). We must select a distributed tracing backend for OTLP span ingestion and trace visualization.

## Decision
NimbusDB will use **Jaeger** (with native OTLP gRPC endpoint on port `4317` and web UI on port `16686`).

## Rationale
1. **Industry Recognition**: Jaeger is a CNCF graduated distributed tracing platform extensively used in Microsoft Azure, Uber, and Kubernetes ecosystems.
2. **Native OTel/OTLP Compatibility**: Accepts standard OpenTelemetry gRPC OTLP trace exports (`go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc`).
3. **Seamless Span Search**: Enables querying request traces directly by `trace_id` and visualizing nested RPC parent-child spans without additional complex infrastructure.

## Implications
- All Go and Rust microservices export OpenTelemetry spans to `JAEGER_OTLP_ENDPOINT` (default `localhost:4317`).
- Gateway injects and propagates W3C TraceContext headers (`traceparent`) on all REST and internal gRPC calls.
