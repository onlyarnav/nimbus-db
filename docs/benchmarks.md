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
