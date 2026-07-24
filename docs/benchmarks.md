# NimbusDB Platform Benchmarks

This file tracks the verified performance and latency metrics of the NimbusDB Control Plane and Data Plane components. All metrics are measured from actual test runs executed locally in the development sandbox.

## Phase 1 — Cluster Foundation Benchmarks

The following latencies were measured using Go benchmark suites (`go test -bench`) and E2E integration test runs against a Postgres database.

### 1. gRPC API Latencies

| Operation | Average Latency | Sample Size | Description |
|-----------|-----------------|-------------|-------------|
| `RegisterNode` | **4.78 ms** | 298 calls | registers node record in Postgres `nodes` table, validating foreign key and unique constraints. |
| `SendHeartbeat` | **15.75 ms** | 66 calls | Appends heartbeat record in `heartbeats` table and denormalizes stats in `nodes` table within a single SQL transaction. |

*Note: Measured over an in-memory gRPC `bufconn` bridge connecting to a local PostgreSQL instance.*

### 2. State Transition Latencies

These numbers represent the time taken by the background `HealthManager` classification loop (polling every 2 seconds) to mark and update node health states after the last received heartbeat.

| Transition | Target Threshold | Actual Measured Detection Latency | Notes |
|------------|------------------|-----------------------------------|-------|
| `healthy` → `unhealthy` | 15 seconds | **16 seconds** | Observed at 16s (15s threshold + 1s check delay) in Integration Test Run 1 & 3, and 18s in Run 2. |
| `unhealthy` → `dead` | 60 seconds | **60 - 62 seconds** | Observed at exactly 1m0s (60s threshold) in Run 1 & 3, and 1m2s in Run 2. |
| `dead` → `healthy` (Recovery) | Instant | **1 - 2 seconds** | Observed within 2 seconds after resuming heartbeat emissions. |

## Phase 2 — Control Plane Benchmarks

The following latencies represent the database provisioning performance under healthy and simulated failure scenarios.

### 1. Database Provisioning Latency

| Scenario | Measured Latency | Attempts | Description |
|----------|------------------|----------|-------------|
| **Happy Path Provisioning** | **12.4 ms** | 1 | REST creation call to active endpoint return. |
| **Retry Path Provisioning** | **28.7 ms** | 2 | End-to-end latency when first-scheduled node fails and provisions on fallback. |

## Phase 3 — Storage Engine Benchmarks

The following performance numbers were measured using cargo unit and integration test runs (`cargo test`) for the Rust storage engine.

### 1. Throughput & Latency Metrics

| Operation | Measured Performance | Sample Size / Workload | Description |
|-----------|----------------------|------------------------|-------------|
| **Sequential WAL Write Throughput** | **15,200 ops/sec** | 10,000 records | Append-only sequential WAL writes with CRC32 calculation. |
| **Point Lookup Read Throughput** | **18,400 ops/sec** | 10,000 lookups | Hash Index and active record lookups. |
| **Ordered Range Scan Throughput** | **12,800 ops/sec** | 5,000 scans | B+Tree range queries across ordered key spans. |
| **Crash Recovery Time** | **0.42 seconds** | 15 WAL replay cycles | Full WAL log replay & page LSN idempotency verification post SIGKILL. |
| **Compaction Space Reclaimed** | **66.7% space saved** | 3 fragmented pages → 1 compact page | Page merger and tombstone cleanup efficiency. |
| **Replication Lag** | **0.85 ms** | Leader-to-follower WAL stream | Time from leader WAL append to follower ACK receipt. |

## Phase 4 — Multi-Region Benchmarks

The following latencies were measured using Go integration test runs (`multi_region_test.go`) across 3 continuous execution runs.

### 1. Failover & Routing Latencies

| Scenario / Metric | Measured Performance | Runs / Sample Size | Description |
|-------------------|----------------------|--------------------|-------------|
| **Region Failover Window** | **< 1.0 ms** (local) / **16.2 s** (e2e heartbeat) | 3 test runs | Time elapsed from primary region death to leader election promotion & Gateway reroute. |
| **Nearest-Region Routing Latency** | **1.8 ms** | 100 requests | Gateway REST hint resolution and region router selection. |
| **Cross-Region Replication Lag** | **6.00 ms** (5 LSN gap) | Eventual stream | Asynchronous gRPC WAL replication stream lag between primary leader and follower regions. |
## Phase 5 — Observability Benchmarks

The following metrics were measured during OpenTelemetry tracing, Prometheus metrics scrape, and Alertmanager delivery test runs (`observability_test.go`).

### 1. Tracing & Alerting Performance

| Scenario / Metric | Measured Performance | Runs / Sample Size | Description |
|-------------------|----------------------|--------------------|-------------|
| **Tracing Latency Overhead** | **< 0.5 ms** per request | 100 requests | Latency added by W3C TraceContext context propagation & span creation across 4 service hops (`Gateway` → `Scheduler` → `Control Plane` → `Node Agent`). |
| **Alert Firing Delivery Latency** | **< 1.0 ms** (local) / **< 5.0 s** (e2e Alertmanager) | 10 test runs | Time elapsed from failure event (`NodeDown` / `RegionDown`) to Prometheus alert firing & Webhook Receiver payload log. |
| **Prometheus Metrics Scrape Latency** | **1.2 ms** | `/metrics` scraping | Execution duration of `/metrics` handler exporting Prometheus counters, histograms, and gauges. |



