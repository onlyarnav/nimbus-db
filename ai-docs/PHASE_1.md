# PHASE_1.md — Distributed Cluster Foundation

Companion spec to `GEMINI.md`. This is the complete, self-contained build
document for Phase 1. Follow this step by step — do not skip ahead to
Phase 2 concerns (provisioning, actual database creation) even if they seem
related.

---

## 1. Goal of This Phase

Stand up the skeleton of the distributed system: nodes can register
themselves, prove they're alive via heartbeats, get detected when they die
or degrade, and a scheduler can pick the best node for a hypothetical
workload. No actual database provisioning happens yet — that's Phase 2.

**Definition of done:** a cluster of 3+ simulated worker nodes registers
with the Metadata Service, sends heartbeats, one node is killed and is
correctly marked unhealthy within a bounded time, and the Scheduler
correctly ranks nodes by load — all demonstrated via integration test and
visible on a minimal dashboard.

---

## 2. Services Built in This Phase

```
services/
├── metadata-service/     ← source of truth (Go)
├── worker-node/           ← simulated node agent (Go) — heartbeat + fake resource stats
├── scheduler/              ← placement decision service (Go)
└── dashboard/               ← minimal live node view (Next.js)
```

Each is a **separately runnable process**. They talk over HTTP/gRPC — pick
one and be consistent (recommendation: gRPC for internal service-to-service
calls, since it typed and matches Go conventions well; REST/JSON only at
the dashboard's edge).

---

## 3. Metadata Service

### 3.1 Responsibilities
- Single source of truth for cluster topology.
- Owns all schema/tables listed below.
- Exposes gRPC (or REST, if chosen) API for: node registration, heartbeat
  ingestion, health status queries, cluster/region/database CRUD (schema
  only for now — no real databases yet).

### 3.2 Database Choice for the Metadata Store Itself
Use Postgres (matches your existing SQLAlchemy/Postgres experience if you
bridge with Python tooling, but since this service is Go, use `pgx` or
`sqlc` for typed queries — **do not hand-roll raw SQL string concatenation**).
Alternative: SQLite for local dev, Postgres for anything resembling
"production" — confirm which before starting if unsure.

### 3.3 Schema

```sql
CREATE TABLE regions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL UNIQUE,      -- e.g. "india", "us-east"
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE clusters (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL UNIQUE,
    region_id   UUID NOT NULL REFERENCES regions(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE nodes (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id      UUID NOT NULL REFERENCES clusters(id),
    hostname        TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'unknown',  -- enum: healthy | unhealthy | draining | dead | unknown
    cpu_pct         REAL,
    memory_pct      REAL,
    disk_pct        REAL,
    last_heartbeat  TIMESTAMPTZ,
    registered_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (cluster_id, hostname)
);

CREATE TABLE heartbeats (
    id          BIGSERIAL PRIMARY KEY,
    node_id     UUID NOT NULL REFERENCES nodes(id),
    cpu_pct     REAL NOT NULL,
    memory_pct  REAL NOT NULL,
    disk_pct    REAL NOT NULL,
    healthy     BOOLEAN NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- heartbeats is an append-only log; nodes.last_heartbeat / status is the
-- fast-read denormalized current state. Add a retention/cleanup job later
-- (not required in Phase 1, but note it in README as a known gap).

-- Placeholder tables — created now for schema completeness per GEMINI.md,
-- NOT populated with real logic until Phase 2:
CREATE TABLE databases (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    node_id     UUID REFERENCES nodes(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE replicas (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    database_id  UUID NOT NULL REFERENCES databases(id),
    node_id      UUID NOT NULL REFERENCES nodes(id),
    role         TEXT NOT NULL DEFAULT 'follower'  -- leader | follower
);
```

Use `UniqueConstraint`-equivalent (`UNIQUE`) wherever uniqueness is the
actual intent — per project convention, don't substitute a plain index.

### 3.4 API Surface

| Method | Path                         | Purpose                                  |
|--------|-------------------------------|-------------------------------------------|
| POST   | `/v1/nodes/register`          | Worker registers, receives back a NodeID  |
| POST   | `/v1/nodes/{id}/heartbeat`    | Worker sends periodic health payload      |
| GET    | `/v1/nodes`                   | List all nodes + current status           |
| GET    | `/v1/nodes/{id}`              | Single node detail                        |
| GET    | `/v1/clusters`                | List clusters                             |
| POST   | `/v1/clusters`                | Create a cluster (admin/setup use)        |
| GET    | `/v1/regions`                 | List regions                              |
| POST   | `/v1/regions`                 | Create a region (admin/setup use)         |
| GET    | `/health`                     | Service's own liveness probe              |

**Register request/response:**
```json
// POST /v1/nodes/register
{ "clusterId": "...", "hostname": "worker-1" }

// 200 response
{ "nodeId": "...", "heartbeatIntervalSeconds": 5 }
```

**Heartbeat payload:**
```json
{
  "cpu": 42.0,
  "memory": 61.0,
  "disk": 58.0,
  "healthy": true
}
```

---

## 4. Worker Node (simulated)

### 4.1 Lifecycle
```
Start → POST /v1/nodes/register → store NodeID locally
     → every 5s: gather (fake) resource stats → POST heartbeat
     → on SIGTERM: attempt graceful deregister (best-effort, log if it fails)
```

### 4.2 Fake Resource Stats
For Phase 1, do **not** try to read real host CPU/memory (unnecessary
complexity this early). Generate plausible synthetic values with slow
random walk (so dashboards look realistic, not jittery noise). Document
this clearly in the worker's README so nobody mistakes it for real
telemetry later.

### 4.3 Failure Simulation Hook
Worker must support a way to be killed/paused to test Health Manager
detection — e.g., a CLI flag or signal handler that stops sending
heartbeats without exiting the process (simulates "slow/hung node" vs
SIGKILL simulating "dead node"). Both cases must be testable.

---

## 5. Health Manager

Runs inside (or alongside) the Metadata Service. Polls/subscribes to
heartbeat freshness.

**Rules (tune thresholds, but pick concrete numbers and write them down):**
- `healthy`: heartbeat received within last 15s (3x the 5s interval).
- `unhealthy`: no heartbeat for 15–60s → mark unhealthy, log
  `"Node Down"` event, but do not evict from scheduling pool yet.
- `dead`: no heartbeat for 60s+ → mark dead, evict from scheduler's
  candidate pool.
- `overloaded`: heartbeat received but cpu/memory/disk above a configured
  threshold (e.g. 90%) for N consecutive heartbeats → mark `overloaded`,
  deprioritize (not exclude) in scheduling.

These specific thresholds are a placeholder starting point — reasonable
defaults, not something benchmarked. Say so if referencing them anywhere
resume-facing.

---

## 6. Scheduler

### 6.1 Phase 1 Algorithm: Least Loaded
Given a hypothetical placement request (no real DB creation yet — this can
be tested via a `POST /v1/schedule/test` debug endpoint that just returns
the chosen node), the scheduler:
1. Filters out `dead` and `draining` nodes.
2. Deprioritizes `overloaded` nodes (only picked if nothing else available).
3. Scores remaining nodes by a simple weighted formula, e.g.:
   `score = (100 - cpu_pct) * 0.4 + (100 - memory_pct) * 0.3 + (100 - disk_pct) * 0.3`
4. Picks the highest score.

Document the formula in code comments and README — this is a deliberately
simple v1; Phase 4 will need cost-based/anti-affinity variants, don't
over-engineer this now.

### 6.2 API
| Method | Path                  | Purpose                                    |
|--------|------------------------|----------------------------------------------|
| POST   | `/v1/schedule/test`   | Debug endpoint: given fake constraints, return chosen nodeId + score breakdown |

---

## 7. Dashboard (minimal)

Next.js page, polling `GET /v1/nodes` every few seconds (no need for
websockets in Phase 1 — that's a nice-to-have, not required).

**Must show, per node:**
- hostname, cluster, region
- status (color-coded: healthy/unhealthy/dead/overloaded)
- cpu/memory/disk %
- last heartbeat timestamp (relative, e.g. "3s ago")

No auth, no styling polish required — function over form for Phase 1.

---

## 8. Testing Requirements (do not skip)

### 8.1 Unit Tests
- Health Manager: given a set of last-heartbeat timestamps, correctly
  classifies healthy/unhealthy/dead per the thresholds in Section 5.
- Scheduler: given synthetic node states, correctly ranks and excludes
  dead/draining nodes; correctly deprioritizes overloaded nodes.
- Metadata Service: registration produces a unique NodeID; duplicate
  hostname within a cluster is rejected per the `UNIQUE` constraint.

### 8.2 Integration Test (required, this is the Phase 1 acceptance proof)
Spin up: 1 Metadata Service + 3 worker nodes (real processes or
docker-compose) against a real (or test-container) Postgres.
1. Confirm all 3 register and appear as `healthy` within 15s.
2. Kill worker 2's heartbeat (pause, not exit).
3. Assert it transitions to `unhealthy` then `dead` at the correct time
   boundaries (allow reasonable test tolerance, e.g. ±5s).
4. Call `POST /v1/schedule/test` and assert the dead node is never
   returned as the chosen node.
5. Resume worker 2's heartbeats, assert it returns to `healthy`.

This test script/file becomes the demoable artifact for interviews —
keep it clean and runnable with a single command (e.g.
`make test-integration` or `docker-compose -f test.yml up --abort-on-container-exit`).

### 8.3 What "done" looks like
- [ ] All unit tests pass
- [ ] Integration test passes reliably (run it 3x to check for flakiness —
      heartbeat-timing tests are a classic source of flaky tests; fix or
      document any flakiness before marking done)
- [ ] Dashboard shows live status of a running cluster
- [ ] `docs/benchmarks.md` updated with: node registration latency,
      heartbeat processing latency, dead-node-detection latency — all
      measured from the integration test run, not estimated
- [ ] Each service has a README (what it does, how to run it, known gaps)
- [ ] `docs/decisions/` updated if Postgres vs SQLite or gRPC vs REST was
      decided during this phase

---

## 9. Explicit Non-Goals for Phase 1

Do not build these now — they belong to later phases, and building them
early violates the "no scope creep" principle in `GEMINI.md`:
- Actual database creation/provisioning (Phase 2)
- Any real storage engine (Phase 3)
- Multi-region routing logic (Phase 4)
- Metrics/tracing pipeline beyond basic `/health` (Phase 5)
- Auth/RBAC (Phase 8)

---

## 10. Suggested Build Order (within this phase)

1. Metadata Service schema + migrations, `/health` endpoint.
2. Node registration endpoint + worker registration client.
3. Heartbeat endpoint + worker heartbeat loop.
4. Health Manager classification logic + unit tests.
5. Scheduler (Least Loaded) + unit tests.
6. Integration test (Section 8.2) — this is the phase's real milestone.
7. Dashboard (can be built in parallel with step 6 by re-using the same
   `/v1/nodes` endpoint).
8. Benchmarks + READMEs + decision docs — close out the phase.

Work through these one at a time. After each numbered step, report status
before moving to the next, per the working agreement in `GEMINI.md`.
