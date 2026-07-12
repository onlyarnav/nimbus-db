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
| PHASE_2.md            | ✅ Written  | Control Plane                                     |
| PHASE_3.md               | ✅ Written  | Storage Engine                                     |
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
| 1 — Cluster Foundation | ✅ Complete | 2026-07-12 | 2026-07-13 | Skeleton, migrations, and health check verified. |
| 2 — Control Plane        | ⬜ Not started | — | — | Ready to begin. |
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
| Postgres vs SQLite for metadata store (dev) | Phase 1, Step 1 | ✅ Resolved | [metadata-store-choice.md](file:///d:/nimbus-db/docs/decisions/metadata-store-choice.md) |
| gRPC vs REST for internal service calls | Phase 1, Step 1 | ✅ Resolved | [internal-rpc-choice.md](file:///d:/nimbus-db/docs/decisions/internal-rpc-choice.md) |
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

**Phase 1, Step 1** (per `PHASE_1.md` Section 10):
- Resolve the two open Phase 1 decisions (Section 3 above).
- Build Metadata Service skeleton + Postgres schema/migrations + `/health`
  endpoint.
- Do not proceed to Step 2 (node registration) until Step 1 is reported
  and approved.

---

## 7. Session Log

*(Append one line per work session — keep it terse.)*

- 2026-07-13 — Spec phase: GEMINI.md, PHASE_1.md, PHASE_2.md, PHASE_3.md,
  PROJECT_STATUS.md written. No code yet. Next: begin Phase 1 build.
- 2026-07-13 — Phase 1 Step 1 complete. Audited and verified Postgres database choice and gRPC architectural target.

