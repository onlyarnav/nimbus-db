# Architectural Decision Record: Dashboard Visualization Approach

## Context
Phase 5 requires rich visualization charts for cluster/node/region health, request latencies (p50/p95/p99), traffic RPS, error rates, and replication lag over time. We must choose between building custom charts vs embedding Grafana dashboards.

## Decision
NimbusDB will **embed Grafana Dashboards via iframe** alongside the Next.js control plane UI, while providing Prometheus API fallback fetching for direct UI telemetry.

## Rationale
1. **Production Authenticity**: Grafana is the industry-standard visualization tool used in enterprise cloud platforms (e.g. Azure Monitor / Grafana managed service).
2. **Speed & Richness**: Avoids reinventing complex time-series plotting logic, heatmaps, and latency histograms from scratch in JavaScript.
3. **Trace-to-Metric Linking**: Allows direct drill-down from Grafana metric spikes to Jaeger traces.

## Implications
- Grafana is provisioned via `deploy/grafana/` with pre-configured Prometheus and Jaeger datasources.
- Next.js dashboard embedded view displays live Grafana panels.
