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
| 3 — Storage Engine          | ⬜ Not started | — | — | Blocked on Phase 2. Language decision (Rust vs C++) pending. |
| 4 — Multi-Region                | ⬜ Not started | — | — | Blocked on Phase 3. Consistency model decision pending. |
| 5 — Observability                  | ⬜ Not started | — | — | Blocked on Phase 4. |
| 6 — AI-Ready Database                  | ⬜ Not started | — | — | Blocked on Phase 5. HNSW vs IVF decision pending. |
| 7 — Cloud Operations                       | ⬜ Not started | — | — | Blocked on Phase 6. |
| 8 — Security                                  | ⬜ Not started | — | — | Blocked on Phase 7. Endpoint audit required as pre-work. |
| 9 — Kubernetes Deployment                        | ⬜ Not started | — | — | Blocked on Phase 8. StatefulSet vs Deployment decision pending. |
| 10 — CI/CD                                          | ⬜ Not started | — | — | Blocked on Phase 9. Final phase. |

Status values: ⬜ Not started · 🟡 In progress · ✅ Complete · 🔴 Blocked

**Reminder (established after the earlier Phase 1 status correction):** a
phase is only marked ✅ Complete once every step in its "Suggested Build
Order" AND every item in its "What done looks like" checklist are
finished and verified — never on partial progress.

---

## 3. Open Decisions (must be resolved, tracked until logged in `docs/decisions/`)

| Decision | Needed by | Status | Resolution |
|----------|-----------|--------|------------|
| Postgres vs SQLite for metadata store (dev) | Phase 1, Step 1 | ✅ Resolved | `docs/decisions/metadata-store-choice.md` |
| gRPC vs REST for internal service calls | Phase 1, Step 1 | ✅ Resolved | `docs/decisions/internal-rpc-choice.md` |
| Rust vs C++ for storage engine | Phase 3, before any code | ✅ Resolved | `docs/decisions/rust-vs-cpp.md` |
| WAL fsync policy (every write vs batched) | Phase 3, Section 5.1 | ⬜ Pending | — |
| Replication ACK quorum policy | Phase 3, Section 10.1 | ⬜ Pending (default: ACK from all followers, refine in Phase 4) | — |
| Consistency model: eventual vs strong | Phase 4, Section 3 | ⬜ Pending | — |
| ANN index: HNSW vs IVF | Phase 6, Section 4.2 | ⬜ Pending (recommendation: HNSW, ties to Qdrant history) | — |
| Auth approach: JWT bearer vs full OAuth2 | Phase 8, Section 4.1 | ⬜ Pending (recommendation: JWT bearer) | — |
| K8s workload types (StatefulSet vs Deployment per service) | Phase 9, Section 6.2 | ⬜ Pending | — |
| HPA vs Phase 7's app-level autoscaler | Phase 9, Section 6.1 | ⬜ Pending (recommendation: both, different layers) | — |
| Release strategy: merge-to-main vs tag-based | Phase 10, Section 4.1 | ⬜ Pending | — |

Once resolved, each decision gets a file in `docs/decisions/` and this
table row updates to ✅ with a link/reference.

---

## 4. Deferred / Cross-Phase Items (do not lose track of these)

| Item | Originally scoped in | Actually resolved in | Status |
|------|------------------------|------------------------|--------|
| Real backup/restore (Node Agent) | Phase 2 (stub only) | Phase 3, Section 8 | ⬜ Pending — closes once Phase 3 Section 8.2 test passes |
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

**Phase 3 — Storage Engine**:
- Design and build 4KB page size structures and memory mapping helpers.
- Design Write-Ahead Log (WAL) records serialization, recovery replay logic.
- Design Hash Index and B+Tree paging layout in Rust.
- Set up leader/follower ACK-based commit replication tests.

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