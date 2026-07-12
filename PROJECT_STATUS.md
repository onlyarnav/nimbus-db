# PROJECT_STATUS.md — NimbusDB Running Status

Single source of truth for "where are we." Update this file at the end of
every session — every phase file (`PHASE_N.md`) ends with an instruction
to update this. If this file is out of date, trust it less than a fresh
read of the actual repo state.

Last updated: 2026-07-13

---

## 1. Spec Files Status

| File           | Status     | Notes                                          |
|-----------------|------------|--------------------------------------------------|
| GEMINI.md        | ✅ Written  | Project constitution — architecture, principles, conventions |
| PHASE_1.md         | ✅ Written  | Cluster Foundation                               |
| PHASE_2.md            | ✅ Written  | Control Plane — revised to correct REST→gRPC drift for internal calls |
| PHASE_3.md               | ✅ Written  | Storage Engine — revised, protocol note added confirming inherited gRPC contract |
| PHASE_4.md                  | ⬜ Not written | Multi-Region                                       |
| PHASE_5.md                     | ⬜ Not written | Observability                                       |
| PHASE_6.md                        | ⬜ Not written | AI-Ready Database                                    |
| PHASE_7.md                           | ⬜ Not written | Cloud Operations                                      |
| PHASE_8.md                              | ⬜ Not written | Security                                                |
| PHASE_9.md                                 | ⬜ Not written | Kubernetes Deployment                                    |
| PHASE_10.md                                   | ⬜ Not written | CI/CD                                                       |
| PROJECT_STATUS.md                                | ✅ Written (this file) | |

---

## 2. Build Status

| Phase | Status         | Started | Completed | Notes |
|-------|-----------------|---------|-----------|-------|
| 1 — Cluster Foundation | 🟡 In progress | 2026-07-12 | — | **Corrected from "Complete."** Only Step 1 of 8 (PHASE_1.md Section 10) is done: Metadata Service skeleton, Postgres schema/migrations, `/health` endpoint — plus a retroactive audit confirming Postgres + gRPC decisions. Steps 2–8 (node registration, heartbeats, Health Manager, Scheduler, integration test, dashboard, benchmarks/READMEs) are still outstanding. Do not report this phase as complete until all 8 steps and the full acceptance checklist in PHASE_1.md Section 8.3 are done. |
| 2 — Control Plane        | ⬜ Not started | — | — | Blocked on Phase 1 completion. Spec now correctly specifies gRPC for all internal calls. |
| 3 — Storage Engine          | ⬜ Not started | — | — | Blocked on Phase 2. Language decision (Rust vs C++) pending — must be resolved before this starts. |
| 4 — Multi-Region                | ⬜ Not started | — | — | Blocked on Phase 3. Spec not yet written. |
| 5 — Observability                  | ⬜ Not started | — | — | Spec not yet written. |
| 6 — AI-Ready Database                  | ⬜ Not started | — | — | Spec not yet written. |
| 7 — Cloud Operations                       | ⬜ Not started | — | — | Spec not yet written. |
| 8 — Security                                  | ⬜ Not started | — | — | Spec not yet written. |
| 9 — Kubernetes Deployment                        | ⬜ Not started | — | — | Spec not yet written. |
| 10 — CI/CD                                          | ⬜ Not started | — | — | Spec not yet written. |

Status values: ⬜ Not started · 🟡 In progress · ✅ Complete · 🔴 Blocked

---

## 3. Open Decisions (must be resolved, tracked until logged in `docs/decisions/`)

| Decision | Needed by | Status | Resolution |
|----------|-----------|--------|------------|
| Postgres vs SQLite for metadata store (dev) | Phase 1, Step 1 | ✅ Resolved | `docs/decisions/metadata-store-choice.md` |
| gRPC vs REST for internal service calls | Phase 1, Step 1 | ✅ Resolved | `docs/decisions/internal-rpc-choice.md` |
| Rust vs C++ for storage engine | Phase 3, before any code | ⬜ Pending | — |
| WAL fsync policy (every write vs batched) | Phase 3, Section 5.1 | ⬜ Pending | — |
| Replication ACK quorum policy | Phase 3, Section 10.1 | ⬜ Pending (default: ACK from all followers, refine in Phase 4) | — |
| Consistency model: eventual vs strong | Phase 4 | ⬜ Pending | — |

Once resolved, each decision gets a file in `docs/decisions/` and this
table row updates to ✅ with a link/reference.

---

## 4. Deferred / Cross-Phase Items (do not lose track of these)

| Item | Originally scoped in | Actually resolved in | Status |
|------|------------------------|------------------------|--------|
| Real backup/restore (Node Agent) | Phase 2 (stub only) | Phase 3, Section 8 | ⬜ Pending — closes once Phase 3 Section 8.2 test passes |
| Rubrik-style extensions (WORM/immutable snapshots, anomaly detection, legal-hold retention) | Suggested externally, 2026-07-13 | Not scheduled — off-roadmap | Parked. Not aligned with current target list (Google, Microsoft Azure Data Eng, Stripe, Wells Fargo, Rippling). Revisit only if a data-protection/backup-focused company becomes an actual target. |

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

**Phase 1, Step 2** (per `PHASE_1.md` Section 10):
- Step 1 is done and audited (Metadata Service skeleton, Postgres schema/
  migrations, `/health` endpoint, Postgres + gRPC confirmed).
- Build: node registration endpoint (gRPC, per the now-corrected internal
  protocol) + worker registration client.
- Do not proceed to Step 3 (heartbeat loop) until Step 2 is reported and
  approved.
- Do not mark Phase 1 "Complete" in Section 2 until Steps 2–8 are done and
  PHASE_1.md Section 8.3's full checklist passes — completion status was
  corrected this session after being marked complete on Step 1 alone.

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
  Step 1 only. Reverted to "In progress" — Steps 2–8 still outstanding.
- 2026-07-13 — Phase 1 Step 2 complete. Implemented gRPC RegisterNode on Metadata Service and startup registration client on Worker Node.

