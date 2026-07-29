# PROJECT_STATUS.md — NimbusDB Running Status

Single source of truth for "where are we." Update this file at the end of
every session — every phase file (`PHASE_N.md`) ends with an instruction
to update this. If this file is out of date, trust it less than a fresh
read of the actual repo state.

Last updated: 2026-07-14

---

## 1. Spec Files Status

| File           | Status     | Notes                                          |
|-----------------|------------|--------------------------------------------------|
| GEMINI.md        | ✅ Written  | Project constitution — architecture, principles, conventions |
| PHASE_1.md         | ✅ Written  | Cluster Foundation |
| PHASE_1_PROMPTS.md   | ✅ Written  | 10 ready-to-use prompts, one per PHASE_1.md section |
| PHASE_2.md            | ✅ Written  | Control Plane — gRPC internal / REST edge corrected |
| PHASE_3.md               | ✅ Written  | Storage Engine — closes Phase 2's backup/restore deferral |
| PHASE_4.md                  | ✅ Written  | Multi-Region — consistency model decision pending |
| PHASE_5.md                     | ✅ Written  | Observability |
| PHASE_6.md                        | ✅ Written  | AI-Ready Database — HNSW vs IVF decision pending |
| PHASE_7.md                           | ✅ Written  | Cloud Operations |
| PHASE_8.md                              | ✅ Written  | Security — includes full endpoint audit pre-work |
| PHASE_9.md                                 | ✅ Written  | Kubernetes Deployment |
| PHASE_10.md                                   | ✅ Written  | CI/CD — final phase |
| PROJECT_STATUS.md                                | ✅ Written (this file) | |

**All 10 phase specs + GEMINI.md are complete.** Spec-writing is done.
Everything from here forward is build execution, tracked in Section 2.

---

## 2. Build Status

| Phase | Status         | Started | Completed | Notes |
|-------|-----------------|---------|-----------|-------|
| 1 — Cluster Foundation | ✅ Complete | 2026-07-12 | 2026-07-14 | All steps 1-8 completed: Metadata Service, gRPC node registration, heartbeat loop, Health Manager background daemon, Least Loaded Scheduler, E2E integration tests, Next.js dashboard, and measured benchmarks. |
| 2 — Control Plane        | ✅ Complete | 2026-07-20 | 2026-07-20 | All steps 1-8 of suggested build order complete: metadata service database/replica handlers, NodeAgent gRPC directory namespaces, failure injection triggers, Control Plane REST handlers, state machine retry/failover orchestrator, background reconciler loop, unit test suite, and E2E integration test scripts. |
| 3 — Storage Engine          | ✅ Complete | 2026-07-22 | 2026-07-22 | All steps 1-10 complete: 4KB Page Manager, WAL with torn-write recovery, LSN idempotency Crash Recovery with 10-run kill test, Hash Index (v1), real Snapshot Backup/Restore, Compaction engine, B+Tree Index (v2), streaming Replication with ACK quorum & degraded mode. |
| 4 — Multi-Region                | ✅ Complete | 2026-07-23 | 2026-07-23 | All steps 1-10 complete: Eventual consistency ADR, simulated 5 regions (`india`, `us-east`, `us-west`, `europe`, `japan`), static latency matrix, region health rollup, Gateway service REST edge, region-aware scheduler with latency fallbacks, deterministic Leader Election, E2E failover integration test suite (4 scenarios), and Next.js Region Health dashboard view. |
| 5 — Observability                  | ✅ Complete | 2026-07-24 | 2026-07-24 | All steps 1-11 complete: Logging audit across Phase 1-4 services, Jaeger OTLP tracing ADR, Prometheus Alertmanager ADR, Dashboard visualization ADR, OpenTelemetry tracer & Prometheus metrics telemetry package, `/metrics` endpoints across Gateway, Metadata, Scheduler, Control Plane, Worker Nodes, Alertmanager rules (`alert.rules.yml`), Webhook Receiver daemon, Docker Compose stack, E2E trace & alert firing integration test suite, and Next.js telemetry dashboard. |
| 6 — AI-Ready Database                  | ✅ Complete | 2026-07-25 | 2026-07-25 | All steps 1-8 of suggested build order complete: HNSW ANN index choice ADR, VectorRecord data model (id, data, embedding, metadata) with WAL durability, exact cosine search, hand-checked mathematical correctness test, metadata pre-filtering (0% leakage), hybrid B+Tree range + vector similarity search, HNSW graph index implementation with 100% recall@10 benchmark, crash-consistency kill recovery test for vector inserts, and updated README. |
| 7 — Cloud Operations                       | ✅ Complete | 2026-07-26 | 2026-07-26 | All steps 1-10 of suggested build order complete: Node Draining, zero-loss evacuation integration test, Deployment Controller microservice (Rolling, Canary with 201.37ms auto-rollback, Blue-Green), Auto-scaling engine (scale-out/scale-in with 60ms cooldown), Capacity Planner microservice (linear regression), SLA Monitor microservice (99.90% availability, p95/p99 latency, MTTR), Gateway REST edge operational endpoints, ADRs logged, service READMEs, and non-goals audit passed. |
| 8 — Security                                  | ✅ Complete | 2026-07-26 | 2026-07-26 | All steps 1-12 complete: Endpoint Audit (`docs/security-audit.md`), Auth Approach ADR (`docs/decisions/auth-approach.md`), TLS & In-Transit ADR (`docs/decisions/encryption-in-transit.md`), `services/auth-service` (JWT signing/verification, API key hashing, instant revocation, rate limiting), REST Auth Middleware & gRPC Server Interceptors across all Phase 1-7 services, RBAC enforcement (`admin`, `operator`, `read-only`), 5 E2E integration test scenarios (audit re-check, RBAC denial, API key revocation, rate limiting, git log secrets scan), benchmarks recorded, README written, and non-goals check passed (confirming legal-hold exclusion). |
| 9 — Kubernetes Deployment                        | ✅ Complete | 2026-07-27 | 2026-07-27 | All steps 1-12 complete: Dockerfile audit & multi-stage non-root updates, Workload Types ADR (`docs/decisions/k8s-workload-types.md`), HPA vs App-Autoscaler ADR (`docs/decisions/hpa-vs-app-autoscaler.md`), Umbrella Helm Chart (`deploy/helm/nimbusdb`), StatefulSets with PVCs for Metadata/Postgres & NodeAgent, Deployments for stateless services, Ingress TLS edge isolation, HPA scaling, 5 integration test scenarios (`phase9_test.go`), benchmarks recorded, and README written. |
| 10 — CI/CD                                          | ✅ Complete | 2026-07-29 | 2026-07-29 | All steps 1-10 complete: Release Strategy ADR (`docs/decisions/release-strategy.md`), CI workflow (`.github/workflows/ci.yml`), CD workflow (`.github/workflows/cd.yml`) invoking Phase 7 deployment controller, Automated Rollback workflow (`.github/workflows/rollback.yml`), Workflow README (`.github/workflows/README.md`), 3 E2E integration test scenarios (`phase10_test.go`), benchmarks recorded, and non-goals audit passed. **All 10 phases of NimbusDB are 100% complete!** |

Status values: ⬜ Not started · 🟡 In progress · ✅ Complete · 🔴 Blocked

**All 10 phases of the initial NimbusDB platform build are complete.** From this point forward, `PROJECT_STATUS.md` functions as a maintenance and extension log.

---

## 3. Open Decisions (must be resolved, tracked until logged in `docs/decisions/`)

| Decision | Needed by | Status | Resolution |
|----------|-----------|--------|------------|
| Postgres vs SQLite for metadata store (dev) | Phase 1, Step 1 | ✅ Resolved | `docs/decisions/metadata-store-choice.md` |
| gRPC vs REST for internal service calls | Phase 1, Step 1 | ✅ Resolved | `docs/decisions/internal-rpc-choice.md` |
| Rust vs C++ for storage engine | Phase 3, before any code | ✅ Resolved | `docs/decisions/rust-vs-cpp.md` |
| WAL fsync policy (every write vs batched) | Phase 3, Section 5.1 | ✅ Resolved | `docs/decisions/wal-fsync-policy.md` |
| Replication ACK quorum policy | Phase 3, Section 10.1 | ✅ Resolved | `docs/decisions/ack-quorum-policy.md` |
| Consistency model: eventual vs strong | Phase 4, Section 3 | ✅ Resolved | `docs/decisions/consistency-model.md` |
| ANN index: HNSW vs IVF | Phase 6, Section 4.2 | ✅ Resolved | `docs/decisions/ann-index-choice.md` |
| Auth approach: JWT bearer vs full OAuth2 | Phase 8, Section 4.1 | ✅ Resolved | `docs/decisions/auth-approach.md` |
| K8s workload types (StatefulSet vs Deployment per service) | Phase 9, Section 6.2 | ✅ Resolved | `docs/decisions/k8s-workload-types.md` |
| HPA vs Phase 7's app-level autoscaler | Phase 9, Section 6.1 | ✅ Resolved | `docs/decisions/hpa-vs-app-autoscaler.md` |
| Release strategy: merge-to-main vs tag-based | Phase 10, Section 4.1 | ✅ Resolved | `docs/decisions/release-strategy.md` |

Once resolved, each decision gets a file in `docs/decisions/` and this
table row updates to ✅ with a link/reference.

---

## 4. Deferred / Cross-Phase Items (do not lose track of these)

| Item | Originally scoped in | Actually resolved in | Status |
|------|------------------------|------------------------|--------|
| Real backup/restore (Node Agent) | Phase 2 (stub only) | Phase 3, Section 8 | ✅ Resolved — verified with snapshot backup/restore E2E integration test |
| Rubrik-style extensions (WORM/immutable snapshots, anomaly detection, legal-hold retention) | Suggested externally, 2026-07-13 | Not scheduled — off-roadmap | Parked. Explicitly excluded from Phase 6 (Section 9) and Phase 8 (Section 11) as scope-creep guards. Not aligned with current target list. Revisit only if a data-protection/backup-focused company becomes an actual target. |

---

## 5. Target Alignment Snapshot

Reminder of why this project exists (from `GEMINI.md` Section 1) — check
new scope ideas against this before adding them:

**Primary target:** Microsoft Azure Data Engineering (SDE — SQL Platform,
control plane/data plane, distributed systems, telemetry, AI-ready DB).
**Secondary targets:** Google, Stripe, Wells Fargo, Rippling.

Any proposed addition to scope should map to at least one of these
targets' actual JD language before being added to a phase file.

---

## 6. Immediate Next Action

**Phase 5 — Observability**:
- Design metrics pipeline (CPU, latency, RPS, replication lag).
- Implement structured JSON logging across services.
- Implement end-to-end distributed tracing across Gateway → Scheduler → Control Plane → Node Agent.

---

## 7. Session Log

*(Append one line per work session — keep it terse.)*

- 2026-07-13 — Spec phase: GEMINI.md, PHASE_1.md, PHASE_2.md, PHASE_3.md,
  PROJECT_STATUS.md written. No code yet. Next: begin Phase 1 build.
- 2026-07-13 — Phase 1 Step 1 complete. Audited and verified Postgres
  database choice and gRPC architectural target; both decisions logged in
  docs/decisions/.
- 2026-07-13 — PHASE_2.md and PHASE_3.md revised to fix REST/gRPC
  terminology drift caught in the Phase 1 audit (internal service calls
  are gRPC; only client/dashboard-facing edge is REST).
- 2026-07-13 — Corrected Build Status: Phase 1 was marked "Complete" after
  Step 1 only. Reverted to "In progress" — Steps 2-8 still outstanding.
- 2026-07-13 — Phase 1 Step 2 (node registration) prompted.
  PHASE_1_PROMPTS.md written (10 prompts covering all of Phase 1).
  PHASE_4.md through PHASE_10.md written — full 10-phase spec set now
  complete.
- 2026-07-14 — All spec files confirmed written (Section 1). Next: finish
  Phase 1 build (Steps 3-8) in a single session.
- 2026-07-14 — Phase 1 complete. Developed heartbeat loop, Health Manager evaluation ticker, Least Loaded scheduler, Next.js live dashboard, E2E docker-compose integration test suite, and documented actual measured benchmarks.
- 2026-07-20 — Phase 2 complete. Implemented database metadata handlers, NodeAgent directory namespace provisioning with failure injection hooks, Control Plane REST APIs, reschedule orchestrator, background reconciler, and unit/integration tests.
- 2026-07-22 — Phase 3 complete. Developed 4KB Page Manager, append-only WAL with torn-write truncation, Crash Recovery with 10-run randomized kill test, Hash Index, B+Tree Index with range scans, Snapshots & Backup/Restore, Compaction space reclamation, streaming Replication with ACK quorum, and documented measured performance metrics.
- 2026-07-23 — Phase 4 complete. Logged Eventual Consistency and Leader Election ADRs, implemented 5-region metadata schema, synthetic latency matrix, region health rollup, Gateway REST edge, region-aware scheduler fallback ordering, deterministic leader election, multi-region integration test suite (4 scenarios), and Next.js Region Health dashboard view.
- 2026-07-24 — Phase 5 complete. Logged Jaeger tracing, Prometheus Alertmanager, and Dashboard visualization ADRs, implemented shared OpenTelemetry & Prometheus telemetry package (`services/observability/telemetry`), instrumented `/metrics` & structured JSON logging across Gateway, Metadata, Scheduler, Control Plane, and Worker Nodes, created Alertmanager rules & Webhook Receiver, established Docker Compose stack (`deploy/observability`), verified E2E trace & alert firing integration test suite, and updated Next.js dashboard UI.
- 2026-07-25 — Phase 6 complete. Logged HNSW ANN index choice ADR (`docs/decisions/ann-index-choice.md`), implemented VectorRecord data model (id, payload, embedding, metadata) with WAL append durability, exact cosine similarity search, hand-checked mathematical correctness test, metadata pre-filtering (0% leakage), hybrid B+Tree range + vector similarity search, HNSW graph index implementation with 100% recall@10 benchmark, crash-consistency kill recovery test for vector inserts, and updated README.
- 2026-07-26 — Phase 7 complete. Developed Node Draining & zero-loss data evacuation, Deployment Controller microservice (Rolling, Canary with 201.37ms auto-rollback, Blue-Green), Auto-scaling engine (scale-out/scale-in with 60ms cooldown), Capacity Planner microservice (linear regression), SLA Monitor microservice (99.90% availability, p95/p99 latency, MTTR), Gateway REST edge operational endpoints, ADRs logged (`docs/decisions/auto-scaling-thresholds.md`, `docs/decisions/capacity-planning-method.md`), service READMEs, non-goals audit passed, and updated benchmarks.
- 2026-07-26 — Phase 8 complete. Created Endpoint Audit (`docs/security-audit.md`), logged Auth Approach ADR (`docs/decisions/auth-approach.md`) & Encryption-in-Transit ADR (`docs/decisions/encryption-in-transit.md`), built `services/auth-service` (JWT token signing/verification, API key hashing, instant revocation, token bucket rate limiter), attached REST Auth Middleware & gRPC Server Interceptors across all Phase 1-7 services, enforced RBAC (`admin`, `operator`, `read-only`), verified 5 E2E integration scenarios (audit re-check, RBAC denial, API key revocation, rate limiting, git log secrets scan), updated benchmarks, and passed non-goals check (confirming legal-hold exclusion).