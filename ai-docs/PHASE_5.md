# PHASE_5.md — Observability

Companion spec to `GEMINI.md`, `PHASE_1.md`–`PHASE_4.md`. Do not start
this phase until Phase 4's acceptance criteria are fully met (see
`PROJECT_STATUS.md`). This phase assumes a working multi-region cluster
with region-aware scheduling and failover already in place.

**Important framing note:** `GEMINI.md` Section 2, item 7 already requires
every service to expose `/health` and structured JSON logs *from Phase 1
onward*. This phase does not invent observability from scratch — it turns
what already exists (scattered `/health` endpoints, ad hoc logs) into a
proper metrics/logging/tracing pipeline with a real dashboard and
alerting. If any earlier-phase service is missing structured logging,
fix that as part of this phase's groundwork rather than treating it as a
new requirement.

**Protocol note:** Metric/log/trace ingestion between services and the
observability backend follows the same internal-gRPC convention where
Claude is building custom ingestion; if using OpenTelemetry's standard
exporters (recommended, see Section 3), follow OTel's own wire protocol
(OTLP, which is itself gRPC-based) rather than reinventing one.

---

## 1. Goal of This Phase

Give the cluster real production-style observability: every service emits
metrics, structured logs, and distributed traces; a dashboard visualizes
cluster/node/region health, latency, traffic, and failures; alerts fire on
SLA-relevant thresholds.

**Definition of done:** a single client request can be traced end-to-end
by trace ID across gateway → scheduler → control plane → node agent →
storage and back, visible in the dashboard; a latency spike or node
failure triggers a real alert (webhook/email); metrics/logs are queryable
after the fact, not just visible in real time.

---

## 2. Services Touched in This Phase

```
services/
├── observability/     ← NEW: metrics/log/trace collection + alerting (uses existing tools, not built from scratch)
├── dashboard/            ← EXTENDED: real charts (latency, traffic, errors), not just node/region status tables
└── (all existing services) ← EXTENDED: instrumented with OpenTelemetry SDK
```

`observability` is mostly **configuration and glue**, not new business
logic — Prometheus, Grafana, and an OTel Collector are industry-standard
tools; this phase is about correctly wiring existing services into them,
not reimplementing a metrics system. Building a custom metrics pipeline
from scratch would be lower resume value than demonstrating correct use
of the standard stack Microsoft/most companies actually run.

---

## 3. Stack Choice

- **Metrics:** Prometheus (pull-based scraping of each service's
  `/metrics` endpoint) + Grafana (dashboards).
- **Logs:** structured JSON logs (already required since Phase 1) shipped
  to a simple aggregator — for this project's scale, a lightweight option
  like Loki (pairs naturally with Grafana) is sufficient; do not over-
  engineer this into a full ELK stack unless you specifically want that
  resume line.
- **Traces:** OpenTelemetry SDK in every service, exported via OTLP to an
  OTel Collector, visualized in Grafana (Tempo) or Jaeger — pick one and
  document it in `docs/decisions/tracing-backend.md`.
- **Alerting:** Grafana Alerting (built-in, avoids standing up a separate
  Alertmanager unless you want that specific line item) or Prometheus
  Alertmanager — pick one, document the choice.

This keeps the whole stack to tools that are directly recognizable on a
resume (Prometheus, Grafana, OpenTelemetry) rather than a custom
implementation that would take longer to build and be less
interview-legible.

---

## 4. Metrics

### 4.1 Required Metrics Per Service (minimum set)
Every service exposes a `/metrics` endpoint (Prometheus format):

| Metric | Type | Applies to |
|--------|------|------------|
| `request_duration_seconds` | Histogram | All services with an API surface |
| `requests_total` | Counter (labeled by status) | All services |
| `errors_total` | Counter (labeled by error type) | All services |
| `cpu_usage_percent`, `memory_usage_percent`, `disk_usage_percent` | Gauge | Node Agent (already collected since Phase 1 heartbeats — expose the same data as Prometheus metrics too, don't duplicate collection logic) |
| `replication_lag_seconds` | Gauge | Node Agent (already measured in Phase 3/4 — expose via Prometheus) |
| `active_connections` | Gauge | Gateway |
| `region_health` | Gauge (0=down, 1=degraded, 2=healthy) | Metadata Service |

### 4.2 Wiring
- Reuse data already being collected (heartbeat stats from Phase 1,
  replication lag from Phase 3/4) — expose it via a `/metrics` endpoint
  rather than recomputing it separately. Duplicated collection logic is a
  known anti-pattern; flag it if it happens.
- Use the official Prometheus Go client library for instrumentation.

---

## 5. Structured Logs

### 5.1 Required Log Events (minimum set, matches original spec)
- `provision_started`, `database_created`, `database_create_failed`
- `node_down`, `node_recovered`
- `replication_failed`, `replication_recovered`
- `region_down`, `region_failover_triggered`, `region_recovered`

### 5.2 Log Format
JSON, one event per line, minimum fields: `timestamp`, `service`, `level`,
`event`, `traceId` (once tracing is wired in, Section 6), plus
event-specific fields (e.g. `nodeId`, `region`, `databaseId`).

**Audit against earlier phases:** if any service from Phases 1–4 is
logging with `fmt.Println`/unstructured text instead of JSON, retrofit it
here as part of this phase's scope — this is exactly the kind of gap
`GEMINI.md`'s "documented as it's built, not retrofitted" principle warns
about, so call it out explicitly if found rather than quietly patching it.

---

## 6. Distributed Tracing

### 6.1 Instrumentation
- OpenTelemetry SDK in every service (Go: `go.opentelemetry.io/otel`).
- Propagate trace context via gRPC metadata on every internal call
  (Control Plane → Scheduler, Control Plane → Node Agent, Gateway →
  Control Plane, cross-region replication calls) — this is why the
  gRPC-everywhere-internal decision from Phase 1 pays off here: consistent
  trace propagation would be far messier with a mixed REST/gRPC internal
  surface.
- REST edge (Gateway's client-facing endpoints) also gets instrumented —
  the trace starts here.

### 6.2 Required Trace Spans (matches original spec's flow)
```
Request → Gateway → Scheduler → Node → Storage → Response
```
Each hop is its own span, correctly parented, so the full request path is
visible as a single trace in the tracing UI.

### 6.3 Acceptance
- A single client request produces one trace with spans for every service
  it touched, correctly ordered and timed, with no orphaned/disconnected
  spans.

---

## 7. Dashboard

### 7.1 Extends Phase 4's Dashboard
Phase 1 gave node status, Phase 4 gave region status. This phase adds real
charts:
- Cluster health overview (nodes/regions healthy vs degraded vs down)
- Latency (p50/p95/p99, from `request_duration_seconds`)
- Traffic (RPS over time)
- Error rate (from `errors_total`)
- Replication lag over time
- A trace search/lookup view (search by trace ID, see the full span tree)

### 7.2 Build vs Embed Decision
For charts, either build custom panels in the Next.js dashboard (using
`recharts`, consistent with `GEMINI.md`'s dashboard stack) querying
Prometheus directly, **or** embed Grafana dashboards via iframe. Embedding
Grafana is faster and arguably more resume-authentic (shows you can use
the standard tool, not just chart libraries) — recommended default unless
you specifically want the custom-dashboard-building exercise. Document
whichever is chosen.

---

## 8. Alerting

### 8.1 Required Alert Rules (minimum set, matches original spec)
- Latency > 500ms (p95) sustained for N minutes → webhook/email
- Node down → webhook/email (this duplicates Phase 1's Health Manager
  detection but now surfaces it through the standard alerting pipeline
  instead of only internal state — document this as intentional, not
  redundant)
- Region down → webhook/email (high severity)
- Replication lag exceeding a threshold → webhook/email
- Error rate exceeding a threshold → webhook/email

### 8.2 Delivery
A simple webhook receiver (can be a mock/logging endpoint for this
project — do not spend time integrating a real Slack/PagerDuty account
unless you already have one you want wired in) that logs received alerts,
proving the pipeline actually fires end-to-end.

---

## 9. Testing Requirements

### 9.1 Unit Tests
- Metrics instrumentation: given a request, confirm the correct
  counters/histograms are incremented (use Prometheus client library's
  testutil package).
- Log formatting: confirm required fields are present and valid JSON for
  each event type in Section 5.1.

### 9.2 Integration Test (required — this is the phase's acceptance proof)
1. **End-to-end trace test:** issue a client request through the Gateway,
   query the tracing backend by trace ID, assert all expected spans
   (Section 6.2) are present, correctly ordered, with no gaps.
2. **Alert firing test:** trigger a condition that should alert (e.g., use
   Phase 1's failure injection to kill a node), assert the alert fires and
   is received by the webhook receiver within a bounded time.
3. **Dashboard data test:** confirm metrics emitted during a test run are
   queryable in Prometheus/Grafana (or the embedded equivalent) shortly
   after — this can be a manual verification step documented with a
   screenshot rather than a strict automated assertion, since dashboard
   rendering itself is hard to unit test meaningfully.

### 9.3 What "done" looks like
- [ ] All unit tests pass
- [ ] End-to-end trace test passes — a real trace ID can be looked up and
      shows the full request path
- [ ] Alert firing test passes for at least the node-down and latency
      cases
- [ ] Every service from Phases 1–4 confirmed to expose `/metrics` and
      emit structured JSON logs (audit and fix gaps, per Section 5.2)
- [ ] `docs/benchmarks.md` updated with: tracing overhead (latency added
      by instrumentation, measured before/after), alert firing latency
      (time from condition to webhook receipt)
- [ ] `docs/decisions/tracing-backend.md` and alerting-tool choice logged
- [ ] Dashboard shows real charts (Section 7), not just status tables
- [ ] `PROJECT_STATUS.md` updated to mark Phase 5 complete — only once
      every item above is actually done

---

## 10. Explicit Non-Goals for Phase 5

- Log retention/archival policy design (acceptable to keep logs
  ephemeral/short-retention for this project; note as a known gap)
- Full ELK-stack-grade log search (Loki's simpler label-based querying is
  sufficient)
- Anomaly detection or ML-driven alerting (that's a Phase 6 concern, and
  only in the AI-search context already scoped there — not observability
  alerting)
- Auth on dashboard/observability endpoints (Phase 8)
- Cost optimization of the observability stack itself (out of scope for a
  portfolio project)

---

## 11. Suggested Build Order (within this phase)

1. Audit Phases 1–4 for structured-logging gaps (Section 5.2) — fix first,
   before building new pipeline on top of inconsistent foundations.
2. Prometheus client instrumentation in every service (Section 4).
3. OpenTelemetry SDK instrumentation + trace propagation across gRPC calls
   (Section 6.1).
4. Stand up Prometheus + Grafana + tracing backend (Section 3) via Docker/
   docker-compose, wire in all services.
5. Loki (or chosen log aggregator) wired to structured logs.
6. End-to-end trace test (Section 9.2, item 1) — confirm the pipeline
   actually works before building alerting/dashboard polish on top.
7. Alert rules (Section 8) + alert firing test.
8. Dashboard charts (Section 7).
9. Benchmarks + decision docs + `PROJECT_STATUS.md` update.

Work through these one at a time. After each numbered step, report status
before moving to the next, per the working agreement in `GEMINI.md`.
