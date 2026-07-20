# GEMINI.md — Project Constitution: NimbusDB
### A Distributed, AI-Ready Cloud Database Platform

This file is the persistent context for every agentic session on this project.
Read this in full before generating any code. Follow it exactly. If a request
from the user conflicts with this file, flag the conflict before proceeding —
do not silently deviate.

---

## 1. Project Identity

**Codename:** NimbusDB
**One-line pitch:** A multi-region, horizontally scalable database platform with
its own control plane, storage engine, and AI-native vector search — built to
demonstrate the exact engineering surface area of Azure SQL / Cosmos DB /
Azure Data Factory teams at Microsoft.

**Why this project exists (do not lose sight of this):**
This is being built specifically to produce verifiable, defensible engineering
depth for internship/new-grad applications targeting Microsoft Azure Data
Engineering, and secondarily Stripe, Wells Fargo, and Rippling. Every phase
must produce something that can be:
1. Demoed live in an interview.
2. Explained at the systems-design whiteboard level.
3. Backed by a real, measured benchmark number — never an invented one.

**Owner:** Arnav Purohit — B.Tech CSE, JUET. GitHub: `onlyarnav`.
Prior relevant work: Qdrant vector DB bug fix (duplicate point ID handling),
Apache Airflow SparkSubmitOperator strategy-pattern refactor, production
quant research platform (SQLAlchemy 2.0 / pydantic-settings conventions),
Gridixa AI Olympiad proctoring platform (NestJS/Go/FastAPI/Next.js/Postgres/
Redis Streams/MinIO, 1,000 concurrent session target).

---

## 2. Non-Negotiable Engineering Principles

These apply across **every** phase, every service, every commit.

1. **No fabricated metrics, ever.** Every number that appears in code comments,
   READMEs, or generated resume bullets must come from an actual benchmark
   run in this repo. If a number isn't measured yet, leave a placeholder like
   `[BENCHMARK: write throughput pending]` — never invent a plausible-looking
   figure.
2. **Correctness before breadth.** A phase is not "done" when the feature
   exists — it's done when it's tested, survives a crash/kill test where
   applicable, and has a written note of known limitations.
3. **No silent scope-narrowing.** If a phase's spec (Section 5) is too large
   for one session, say so explicitly and propose a split — don't quietly
   build a smaller version and present it as complete.
4. **Every phase ships with tests.** Minimum: unit tests for core logic,
   one integration test exercising the phase's primary flow end-to-end.
   Phase 3 (Storage Engine) additionally requires a crash-recovery test.
5. **Idiomatic per-language conventions** (see Section 4) — no generic
   boilerplate that ignores the standard practices of the chosen language/
   framework.
6. **Everything is documented as it's built**, not retrofitted. Each service
   gets a README explaining what it does, how to run it, and what it does
   NOT yet do.
7. **Observability is not an afterthought bolted on in Phase 5.** Every
   service, from Phase 1 onward, must expose a `/health` endpoint and
   structured (JSON) logs. Metrics/tracing instrumentation proper still
   happens in Phase 5, but the hooks exist from day one.
8. **Ask before assuming on ambiguity that affects architecture.** Naming,
   formatting, and minor implementation choices — just pick sensibly and
   move on. Consistency model, replication strategy, storage format —
   confirm before building.

---

## 3. High-Level Architecture

```
                          ┌─────────────────────┐
                          │   API Gateway        │
                          │  (REST, auth, rate   │
                          │   limiting)           │
                          └──────────┬───────────┘
                                     │
                     ┌───────────────┴───────────────┐
                     │        Control Plane           │
                     │  (Scheduler, Provisioner,       │
                     │   Metadata Service)             │
                     └───────────────┬───────────────┘
                                     │
        ┌────────────────────────────┼────────────────────────────┐
        │                            │                             │
┌───────▼────────┐          ┌────────▼────────┐          ┌─────────▼───────┐
│  Node Agent 1   │          │  Node Agent 2    │          │  Node Agent N     │
│  (Data Plane)   │          │  (Data Plane)    │          │  (Data Plane)     │
│  Storage Engine │          │  Storage Engine  │          │  Storage Engine   │
└─────────────────┘          └──────────────────┘          └───────────────────┘

        Cross-cutting: Metadata Service (source of truth) │ Observability Stack
                        (Metrics / Logs / Traces) │ Multi-Region Router
```

**Control plane vs data plane — the core architectural distinction this
project must demonstrate:**
- **Control plane**: decides *what should happen* (which node hosts which
  database, when to fail over, when to scale). Never touches raw data.
- **Data plane**: *does the work* (reads, writes, replication of actual
  data). Never makes placement/scheduling decisions.

This separation must be real in the code — not just a README claim. The
control plane and data plane are separate deployable services/binaries,
communicating over a defined API, never sharing in-process state.

---

## 4. Tech Stack & Per-Service Conventions

| Service                | Language        | Rationale                                      |
|-------------------------|-----------------|-------------------------------------------------|
| Metadata Service        | Go              | Strong concurrency primitives, fast gRPC support |
| Scheduler                | Go              | Same service family, shares types with Metadata  |
| Control Plane API        | Go              | Consistency with scheduler/metadata              |
| Node Agent / Storage Engine | Rust or C++ (pick one, do not mix) | Needs manual memory/layout control for pages, WAL, B+Tree |
| API Gateway               | Go or Node/NestJS | Reuse NestJS familiarity from Olympiad project if preferred |
| Dashboard                 | Next.js + TypeScript | Matches existing frontend experience |
| Observability pipeline    | Prometheus + Grafana + OpenTelemetry | Industry-standard, resume-recognizable |
| Message/coordination      | Redis Streams or NATS | Reuse Redis Streams experience from Olympiad project |
| Deployment                | Docker → Kubernetes (Phase 9) | |
| CI/CD                     | GitHub Actions | |

**Storage Engine Language:** Rust (decided on 2026-07-14, logged in [rust-vs-cpp.md](file:///d:/nimbus-db/docs/decisions/rust-vs-cpp.md)).

**Conventions to enforce (carried over from prior projects — keep consistent):**
- Go: standard `gofmt`/`golangci-lint`, table-driven tests, context propagation
  on every function that does I/O.
- Storage engine (Rust/C++): explicit error types, no panics/exceptions on
  the I/O hot path, RAII/ownership discipline documented per module.
- Any Python tooling: `pydantic-settings` for config, `pathlib` for all path
  handling — never raw string concatenation.
- Any ORM usage: SQLAlchemy 2.0 `Mapped[T]` typed style if Python is used
  anywhere; prefer `UniqueConstraint` over bare `Index` where uniqueness is
  the actual intent.
- Every service: 12-factor config (env vars / config file, never hardcoded
  hosts/ports).

---

## 5. Phase Breakdown (Source of Truth)

This mirrors the plan already agreed with the user. **Do not rewrite or
reorder these phases** — build them in this sequence. Each phase lists
deliverables and the explicit acceptance criteria the agent must self-check
against before declaring the phase complete.

### Phase 1 — Distributed Cluster Foundation
**Build:** Metadata Service (Clusters, Nodes, Regions, Databases, Replicas,
Heartbeats tables), Worker Node registration + 5s heartbeat loop, Health
Manager (dead/slow/overloaded node detection), Scheduler (Least Loaded
algorithm first).
**Acceptance criteria:**
- A node can start, self-register, and get a NodeID.
- Killing a node's heartbeat causes Health Manager to mark it unhealthy
  within a bounded, tested time window.
- Scheduler correctly picks the least-loaded node given synthetic CPU/RAM/
  disk data.
- Dashboard (even minimal) shows live node list + health status.

### Phase 2 — Control Plane
**Build:** `POST /createDatabase` full flow (validate → choose cluster →
choose node → provision → update metadata → return), Provisioner, Node
Agent (Create/Delete/Backup/Restore stubs acceptable initially), metadata
sync, retry-on-node-failure logic.
**Acceptance criteria:**
- Killing the chosen node mid-provision causes a retry onto a different
  node, and this is demonstrated by an integration test, not just claimed.

### Phase 3 — Storage Engine (hardest, highest-value phase)
**Build:** 4KB paged storage, Write-Ahead Log, crash recovery via WAL
replay, Hash Index (then B+Tree), compaction, periodic snapshots,
leader/follower replication with ACK-based commit.
**Acceptance criteria:**
- A crash-consistency test: kill the process mid-write, restart, verify
  no corruption and correct WAL replay.
- A replication test: write to leader, verify follower converges and ACKs
  before commit is reported successful.
- This phase gets a **measured** write-throughput and recovery-time number.
  No estimates.

### Phase 4 — Multi-Region
**Build:** Multiple simulated regions (India, US East, US West, Europe,
Japan), nearest-region routing, region-aware scheduling (latency,
replication, capacity), region failover, consistency model decision
(eventual vs strong — **confirm with user before implementing**), read
replicas, leader election.
**Acceptance criteria:** killing a region's nodes causes traffic to
reroute to the next-nearest healthy region, demonstrated live.

### Phase 5 — Observability
**Build:** Metrics (CPU, latency, RPS, error rate, memory, replication lag),
structured logs (Provision Started, Database Created, Node Down,
Replication Failed), distributed tracing across gateway → scheduler →
node → storage → response, dashboard (cluster/node health, latency,
traffic, failures, replication), alerting (latency > 500ms → webhook/email).
**Acceptance criteria:** a single request can be traced end-to-end by
trace ID across all services in the dashboard.

### Phase 6 — AI-Ready Database
**Build:** vector storage per record (document → embedding → vector index),
`INSERT VECTOR` / `SEARCH VECTOR`, cosine similarity, approximate search,
metadata filters, hybrid search (SQL predicate + vector similarity
combined).
**Acceptance criteria:** a query like "find documents similar to X filtered
by region=India" returns correct, measurably-ranked results against a real
test dataset.

### Phase 7 — Cloud Operations
**Build:** rolling / canary / blue-green deployment, rollback, node
draining (move databases off before shutdown), auto-scaling (CPU/memory/
traffic based), capacity planner, SLA monitor (99.9% availability,
latency, recovery time tracking).
**Acceptance criteria:** a simulated canary deploy that fails health checks
triggers automatic rollback — demonstrated, not described.

### Phase 8 — Security
**Build:** OAuth, RBAC, API keys, audit logs, encryption at rest, secrets
management, TLS, rate limiting.
**Acceptance criteria:** RBAC must have real role/permission checks
enforced at the API layer with tests proving denial for insufficient
roles — not just a decorative role field.

### Phase 9 — Kubernetes Deployment
**Build:** Helm charts for every service (Gateway, Metadata, Scheduler,
Storage, Telemetry, Dashboard), Ingress, Horizontal Pod Autoscaler.
**Acceptance criteria:** `helm install` brings up a working cluster from
clean state, verified.

### Phase 10 — CI/CD
**Build:** GitHub Actions pipeline — test → build Docker → deploy → health
check → automatic rollback on failure.
**Acceptance criteria:** a deliberately broken commit is caught by the
pipeline and does not reach "deployed" state.

---

## 6. Repository Structure (target)

```
nimbusdb/
├── GEMINI.md                  (this file)
├── docs/
│   ├── architecture.md
│   ├── benchmarks.md          (real numbers only, updated per phase)
│   └── decisions/             (ADRs — one file per major decision, e.g. rust-vs-cpp.md)
├── services/
│   ├── metadata-service/
│   ├── scheduler/
│   ├── control-plane/
│   ├── node-agent/            (storage engine lives here)
│   ├── gateway/
│   └── dashboard/
├── deploy/
│   ├── docker/
│   ├── helm/
│   └── github-actions/
└── tests/
    └── integration/
```

Each `services/<name>/` directory is self-contained: its own README,
its own tests, its own Dockerfile.

---

## 7. Working Agreement for the Agent (Antigravity/Gemini)

- Work **phase by phase, step by step**, as instructed by the user. Do not
  jump ahead to a later phase's code even if it seems convenient.
- Before writing code for a phase, restate that phase's acceptance criteria
  back and confirm scope for the session.
- After finishing a unit of work, report: what was built, what was tested,
  what is explicitly **not** done yet (no silent gaps).
- Never write a README claim, comment, or commit message containing a
  benchmark number that wasn't actually produced by running the code in
  this session.
- Prefer small, reviewable commits over large ones. Each commit should
  correspond to one coherent piece of the current phase.
- When a design decision has real tradeoffs (consistency model, storage
  format, Rust vs C++, sync vs async I/O), stop and ask — log the decision
  and rationale in `docs/decisions/` once made.
- If asked to generate resume bullets from this project, only use metrics
  that exist in `docs/benchmarks.md` — otherwise use bracketed placeholders.

---

## 8. Current Status

**Phase in progress:** Phase 3 — Storage Engine.
**Language decision:** Rust for storage engine (Phase 3) (logged in [rust-vs-cpp.md](file:///d:/nimbus-db/docs/decisions/rust-vs-cpp.md)).
**Next step:** Phase 3 — Storage Engine (4KB page storage, Write-Ahead Log, crash recovery, Hash Index, leader/follower replication).

*(Update this section as phases complete — this is the running state file
for the whole project.)*
