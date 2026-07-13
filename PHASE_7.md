# PHASE_7.md — Cloud Operations

Companion spec to `GEMINI.md`, `PHASE_1.md`–`PHASE_6.md`. Do not start
this phase until Phase 6's acceptance criteria are fully met (see
`PROJECT_STATUS.md`). This phase assumes a working multi-region cluster
with a real storage engine, replication, full observability, and vector
search already in place.

**Protocol note:** Same rule as all prior phases — any new internal
service-to-service calls introduced here (deployment orchestration,
capacity planner queries, SLA monitor) are gRPC. Any operator-facing
control surface (e.g. triggering a manual rollback) goes through the
Gateway's REST edge, consistent with Phase 4.

---

## 1. Goal of This Phase

Turn the cluster from "a system that runs" into "a system that can be
safely operated": rolling/canary/blue-green deployments with automatic
rollback, safe node draining before shutdown, auto-scaling based on real
load signals, a capacity planner, and an SLA monitor tracking the
99.9%-style guarantees the JD language (and Microsoft postings generally)
actually cares about.

**Definition of done:** a deliberately broken canary deployment is
detected via health checks and automatically rolled back without manual
intervention; a node can be drained (databases moved off) and shut down
with zero dropped requests; the system scales node count up/down based on
observed load; the SLA monitor reports real, measured availability and
recovery-time numbers over a test window.

---

## 2. Services Touched in This Phase

```
services/
├── deployment-controller/     ← NEW: rolling/canary/blue-green orchestration, rollback
├── capacity-planner/            ← NEW: predicts node needs from historical load
├── sla-monitor/                    ← NEW: tracks availability/latency/recovery-time SLOs
├── node-agent/                        ← EXTENDED: draining support (move-then-shutdown)
└── scheduler/                            ← EXTENDED: auto-scaling triggers feed into placement decisions
```

These are genuinely new services, not extensions of existing ones — cloud
operations tooling is conceptually distinct from the data-serving path and
should be deployable/failable independently of it (an operations outage
should not take down the database itself).

---

## 3. Deployment Strategies

### 3.1 Rolling Deployment
- Replace nodes/instances of a service one at a time (or in small
  batches), health-checking each before moving to the next.
- If a health check fails during rollout, halt further rollout
  immediately — do not continue replacing healthy instances with an
  unhealthy version.

### 3.2 Canary Deployment
- Deploy the new version to a small subset of nodes (e.g., 1 of N) first.
- Route a small percentage of traffic to the canary (or, in this
  simulated environment, route to it via the Gateway with a configurable
  traffic-split percentage).
- Monitor canary health/error-rate (reuse Phase 5's metrics pipeline —
  do not build a separate monitoring path for this) for a defined
  observation window.
- Promote to full rollout only if the canary's metrics stay within
  acceptable bounds; otherwise auto-rollback (Section 3.4).

### 3.3 Blue-Green Deployment
- Stand up a full parallel ("green") environment alongside the current
  ("blue") one.
- Switch traffic atomically (via Gateway routing config) once green is
  verified healthy.
- Keep blue running for a rollback window before tearing it down.

### 3.4 Automatic Rollback
- Triggered by: failed health checks during rolling deployment, canary
  metrics breaching thresholds, or a manual rollback request.
- Rollback must be as fast and reliable as roll-forward — test this
  explicitly, not just the happy path.

---

## 4. Node Draining

### 4.1 Design
Extends the Node Agent (Phase 2/3) with a `Drain` operation:
```
Drain requested
     ↓
Mark node status = "draining" in Metadata Service (no new placements
  routed here — Scheduler must respect this status, extend Phase 1's
  scheduler filter to exclude "draining" alongside "dead")
     ↓
For each database/replica on this node:
     move to a healthy node (reuse Phase 2's provisioning + Phase 3's
     replication/snapshot mechanism — do not build a new data-movement
     path from scratch)
     ↓
Confirm all data moved and verified
     ↓
Node safe to shut down
```

### 4.2 Acceptance
- Draining a node with active databases results in zero data loss and
  zero failed client requests during the drain (verify via a concurrent
  load-generation script running throughout the drain).

---

## 5. Auto-Scaling

### 5.1 Design
- Triggers: sustained high CPU/memory/traffic across the cluster (reuse
  Phase 5's metrics — do not build separate collection).
- Scale-out: provision new worker nodes (simulated — spinning up a new
  worker-node process/container), register them (Phase 1 flow), let the
  Scheduler start placing new work there.
- Scale-in: identify underutilized nodes, drain them (Section 4), then
  deprovision.
- Define concrete thresholds and a cooldown period (to avoid flapping —
  scaling out and back in repeatedly) and document them as starting
  defaults, same caveat as every other threshold in this project.

### 5.2 Acceptance
- Simulated load spike triggers scale-out within a bounded, measured time.
- Load drop triggers scale-in via drain, with no data loss (reuses
  Section 4's guarantee).

---

## 6. Capacity Planner

### 6.1 Design
- Analyzes historical load data (from Phase 5's metrics store) to predict
  near-term capacity needs (e.g., "+3 nodes needed next week" per the
  original spec's example).
- For this project's scope, a simple trend-based projection (linear
  regression or moving-average growth rate on historical CPU/traffic) is
  sufficient — a full ML forecasting model is disproportionate scope here;
  document this as a deliberate simplification, not a limitation to
  apologize for.
- Output: a report/dashboard panel showing projected capacity need over a
  configurable horizon.

### 6.2 Acceptance
- Given a synthetic historical load dataset with a known growth trend,
  the planner's projection is directionally correct and the specific
  method used is documented (so the number is defensible if asked about
  in an interview, not a black box).

---

## 7. SLA Monitor

### 7.1 Design
- Tracks, over a rolling window: availability (% of time the system
  successfully served requests), latency (p95/p99, reused from Phase 5),
  recovery time (time to recover from injected failures — reuse Phase 1's
  node-kill and Phase 4's region-kill test harnesses).
- Reports against a target SLO (e.g., 99.9% availability) and flags
  breaches.

### 7.2 Acceptance
- Running the existing failure-injection tests (node kill from Phase 1,
  region kill from Phase 4) while the SLA monitor is active produces a
  correct, measured availability/recovery-time report for that test
  window — this is the number that goes in `docs/benchmarks.md`, not an
  estimate.

---

## 8. Testing Requirements

### 8.1 Unit Tests
- Deployment controller: rollout state machine transitions correctly
  (rolling forward, canary evaluation, rollback trigger conditions).
- Node draining: correctly excludes a draining node from new placements
  (Scheduler filter extension).
- Capacity planner: projection math correctness on a known synthetic
  dataset.
- SLA monitor: availability/latency aggregation correctness given
  synthetic request logs.

### 8.2 Integration Test (required — this is the phase's acceptance proof)
1. **Canary rollback test (the core deliverable, matches GEMINI.md
   Section 5 exactly):** deploy a deliberately broken canary version
   (fails health checks) → assert the deployment controller detects it
   and automatically rolls back → assert traffic never fully shifts to
   the broken version → measure and record the detection-to-rollback
   time.
2. **Zero-loss drain test:** run a concurrent load generator against a
   node while draining it → assert zero failed requests and zero data
   loss, verified against Phase 3's crash-consistency-style checks.
3. **Auto-scale test:** simulate a load spike → assert scale-out occurs
   within a bounded time and new capacity is actually used by the
   Scheduler; simulate load drop → assert scale-in drains and
   deprovisions correctly.
4. **SLA report test:** run the existing Phase 1/Phase 4 failure-injection
   tests under the SLA monitor → assert it produces an accurate report
   for that window.

### 8.3 What "done" looks like
- [ ] All unit tests pass
- [ ] All 4 integration scenarios in Section 8.2 pass reliably (run 3x)
- [ ] `docs/benchmarks.md` updated with: canary detection-to-rollback
      time, drain duration + zero-loss confirmation, scale-out/scale-in
      timing, measured SLA report from a real test window — all measured
- [ ] `docs/decisions/` updated with: auto-scaling thresholds/cooldown
      rationale, capacity-planning method choice
- [ ] Each new service (deployment-controller, capacity-planner,
      sla-monitor) has a README
- [ ] `PROJECT_STATUS.md` updated to mark Phase 7 complete — only once
      every item above is actually done

---

## 9. Explicit Non-Goals for Phase 7

- ML-based capacity forecasting (trend-based projection is the documented
  scope boundary, Section 6.1)
- Kubernetes-native deployment mechanics (that's Phase 9 — this phase's
  deployment controller is application-level orchestration, independent
  of the eventual K8s wrapper)
- Auth on any new operational endpoints (Phase 8)
- Cost-optimization logic for scaling decisions (out of scope — scale on
  load signals only, not cost modeling)

---

## 10. Suggested Build Order (within this phase)

1. Node draining (Section 4) — build this first since auto-scaling and
   deployment rollback both depend on safe drain being reliable.
2. Zero-loss drain test (Section 8.2, item 2) — confirm this milestone
   before building on top of it.
3. Rolling deployment (Section 3.1) + health-check-triggered halt.
4. Canary deployment (Section 3.2) + auto-rollback (Section 3.4).
5. Canary rollback test (Section 8.2, item 1) — the phase's core
   deliverable.
6. Blue-green deployment (Section 3.3) — lower priority than canary; build
   after canary is solid.
7. Auto-scaling (Section 5) + scale test (Section 8.2, item 3).
8. Capacity planner (Section 6).
9. SLA monitor (Section 7) + SLA report test (Section 8.2, item 4).
10. Benchmarks + decision docs + READMEs + `PROJECT_STATUS.md` update.

Work through these one at a time. After each numbered step, report status
before moving to the next, per the working agreement in `GEMINI.md`.
