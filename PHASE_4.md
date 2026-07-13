# PHASE_4.md — Multi-Region

Companion spec to `GEMINI.md`, `PHASE_1.md`, `PHASE_2.md`, `PHASE_3.md`.
Do not start this phase until Phase 3's acceptance criteria are fully met
(see `PROJECT_STATUS.md`) — this phase assumes a working single-region
cluster with a real storage engine, WAL, snapshots, and leader/follower
replication already in place.

**Protocol note:** Same rule as Phases 1–3 — all service-to-service calls
introduced in this phase (cross-region routing, region health propagation,
leader election coordination) are **gRPC**. Only the client-facing gateway
edge is REST. No exceptions without logging a new decision.

---

## 1. Goal of This Phase

Take the single-region cluster from Phases 1–3 and make it geo-distributed:
multiple regions, clients routed to the nearest healthy one, region-aware
scheduling, and automatic failover when an entire region goes down — not
just a single node (Phase 1 already handles single-node failure; this
phase is about surviving the loss of a whole region).

**Definition of done:** a client request is routed to the nearest healthy
region; killing all nodes in a region causes traffic to reroute to the
next-nearest healthy region without manual intervention; the chosen
consistency model (Section 3) is implemented correctly and its tradeoffs
are documented, not just asserted.

---

## 2. Services Touched in This Phase

```
services/
├── metadata-service/     ← EXTENDED: regions become live routing targets, not just labels
├── scheduler/               ← EXTENDED: region-aware placement (latency, replication, capacity)
├── control-plane/              ← EXTENDED: region failover orchestration
├── node-agent/                    ← EXTENDED: replication now spans regions, leader election
├── gateway/                          ← NEW: client entry point, nearest-region routing
└── dashboard/                          ← EXTENDED: shows region-level health, not just node-level
```

`gateway` is new — this is the first phase where an explicit "front door"
service makes sense, since clients now need to be routed *to* a region
before anything else happens.

---

## 3. Consistency Model — Decision Gate (must resolve before building)

`GEMINI.md` and `PHASE_3.md` both flag this as pending. Resolve and log in
`docs/decisions/consistency-model.md` before writing region-failover logic.

**Options:**
- **Eventual consistency**: writes to the leader propagate to other
  regions' replicas asynchronously; reads in a follower region may be
  stale for a bounded window. Simpler, lower write latency, matches most
  real multi-region systems' default (including Cosmos DB's default
  consistency level, which is directly relevant to your Microsoft
  targeting).
- **Strong consistency**: reads always reflect the latest committed write,
  requiring cross-region coordination on every write (higher latency,
  simpler reasoning about correctness).

**Recommendation to consider (not a mandate — this is a real tradeoff):**
default to eventual consistency with a documented staleness bound, and
explicitly note this as a deliberate design choice mirroring Cosmos DB's
tunable consistency levels — this is more resume-relevant to your
Microsoft Azure Data Engineering target than forcing strong consistency
everywhere. Whichever is chosen, the reasoning must be written down, not
just implemented.

---

## 4. Regions

### 4.1 Simulated Regions
Five regions, matching the original spec: `india`, `us-east`, `us-west`,
`europe`, `japan`. Each is a self-contained sub-cluster (its own set of
worker nodes from Phase 1, running the full Phase 1–3 stack).

### 4.2 Region Metadata
Regions table already exists (Phase 1, Section 3.3) — this phase makes it
load-bearing:
- Add a synthetic inter-region latency matrix (a static config table for
  simulation purposes — real latency measurement is out of scope for a
  simulated environment; document this clearly as a known simplification).
- Region health is an aggregate: a region is `healthy` if it has at least
  one healthy node; `degraded` if some but not all nodes are unhealthy;
  `down` if all nodes are unhealthy/dead (per Phase 1's Health Manager
  classification, rolled up per region).

---

## 5. Gateway

### 5.1 Responsibilities
- Client-facing entry point (REST, replacing direct calls to
  Control Plane from earlier phases — Control Plane becomes internal-only
  from this phase forward).
- Determines nearest region for an incoming client (use a simple
  client-provided region hint or IP-based approximation — do not build
  real geo-IP infrastructure, that's disproportionate for this project;
  document the simplification).
- Routes the request to that region's Control Plane via gRPC.
- On region failure, reroutes to the next-nearest healthy region
  transparently to the client.

### 5.2 API Surface (client-facing, REST)

| Method | Path                      | Purpose                                  |
|--------|----------------------------|--------------------------------------------|
| POST   | `/v1/databases`            | Create a database (now region-aware)      |
| GET    | `/v1/databases/{id}`       | Get database status/location               |
| GET    | `/v1/regions`               | List regions + health status               |
| GET    | `/health`                    | Gateway's own liveness probe                |

Request now optionally accepts a region hint:
```json
{ "name": "orders-db", "clusterId": "...", "preferredRegion": "india" }
```
If omitted, gateway infers nearest region per Section 5.1. If the
preferred/nearest region is down, gateway must reroute and report which
region actually served the request in the response.

---

## 6. Region-Aware Scheduler

### 6.1 Extends Phase 1's Scheduler
The Least-Loaded algorithm from Phase 1 (`PHASE_1.md` Section 6.1) is
reused *within* a region. This phase adds a region-selection layer on top:

```
Given: preferred/nearest region + fallback list (ordered by latency)
     ↓
Is preferred region healthy (≥1 healthy node)?
     ↓ yes                                    ↓ no
Use Phase 1 scheduler                    Try next region in fallback list
within that region                       (repeat until a healthy region found)
```

### 6.2 Factors Considered (per original spec)
- Latency (from the static matrix in Section 4.2)
- Replication topology (prefer regions that already have a replica of
  relevant data, once cross-region replication exists — see Section 7)
- Capacity (reuse Phase 1's load scoring within the chosen region)

Document this as a simple ordered-fallback policy for Phase 4 — full
cost-based multi-factor scoring is explicitly out of scope here (that's
listed as a "later" refinement in the original project outline); do not
over-build this.

---

## 7. Cross-Region Replication & Leader Election

### 7.1 Extends Phase 3's Replication
Phase 3 built leader/follower replication *within* a region. This phase
extends it across regions:
- One region's node holds the leader role for a given database; other
  regions hold follower replicas.
- Replication across regions uses the same gRPC streaming mechanism as
  Phase 3 (Section 10.1 in `PHASE_3.md`) — do not introduce a different
  protocol for cross-region vs intra-region replication.
- Apply the consistency model decided in Section 3: if eventual, cross-
  region ACKs are not required for commit; if strong, they are (and this
  will materially increase write latency — measure and record it).

### 7.2 Leader Election on Region Failure
- If the leader's region goes down, a follower region's replica must be
  promoted to leader.
- Use a simple deterministic election for this phase (e.g., lowest node ID
  among healthy follower replicas, or a basic term-based election if you
  want to demonstrate Raft-adjacent concepts) — document which approach
  was used and why. A full Raft implementation is not required here
  (that level of rigor belongs in a dedicated consensus-focused project if
  you want one later); this phase needs *correct failover*, not textbook
  consensus.
- After promotion, the new leader must be reachable via the Gateway
  (Section 5) without client-visible errors beyond the failover window
  itself.

---

## 8. Failover Flow (must match this exact sequence — this is the phase's
core deliverable)

```
Region A (leader) goes down (simulated: kill all nodes in that region)
     ↓
Health Manager (Phase 1, per-node) marks all Region A nodes dead
     ↓
Region health aggregate (Section 4.2) rolls Region A up to "down"
     ↓
Leader election (Section 7.2) promotes a follower in Region B (or C)
     ↓
Metadata Service updated: new leader region recorded
     ↓
Gateway's region routing (Section 5.1) stops directing traffic to Region A,
routes to Region B
     ↓
Client requests continue being served (post-failover-window) without
manual intervention
```

---

## 9. Testing Requirements

### 9.1 Unit Tests
- Region health rollup: given a mix of node statuses, correctly computes
  `healthy`/`degraded`/`down` per region.
- Region-aware scheduler fallback: given a down preferred region, correctly
  falls back to the next-nearest healthy region.
- Leader election: given a set of follower candidates, deterministically
  picks the same one every time given the same input (no flakiness).

### 9.2 Integration Test (required — this is the phase's acceptance proof)
Using a multi-region setup (≥3 simulated regions, each with its own
Phase 1–3 stack):

1. **Nearest-region routing:** client request with a region hint routed
   correctly to that region.
2. **Region failover (the core test):** kill every node in the leader's
   region → assert:
   - Region health rolls up to `down` within a bounded time.
   - A follower in another region is promoted to leader.
   - Subsequent client requests (via Gateway) are served successfully by
     the new leader region, with no manual intervention.
   - Measure and record the failover window (time from region death to
     successful client request) — this is the phase's headline number.
3. **Consistency model verification:** depending on Section 3's decision —
   if eventual, demonstrate a bounded staleness window on a follower read
   right after a leader write; if strong, demonstrate a read always
   reflects the latest write even from a follower region.
4. **Recovery:** bring the failed region's nodes back up, confirm they
   rejoin as followers (not incorrectly re-claiming leadership) and
   catch up via replication.

### 9.3 What "done" looks like
- [ ] All unit tests pass
- [ ] Integration test (all 4 scenarios in 9.2) passes reliably (run 3x,
      fix or document flakiness — region-failover timing tests are prone
      to it, same caution as Phase 1's heartbeat tests)
- [ ] `docs/benchmarks.md` updated with: failover window (measured),
      cross-region replication lag, write latency under the chosen
      consistency model — all measured, none estimated
- [ ] Gateway, and updated Scheduler/Control Plane/Node Agent sections,
      have README updates reflecting multi-region behavior
- [ ] `docs/decisions/consistency-model.md` and
      `docs/decisions/leader-election-approach.md` written
- [ ] Dashboard updated to show region-level health
- [ ] `PROJECT_STATUS.md` updated to mark Phase 4 complete — only once
      every item above is actually done, not on partial progress (see the
      Phase 1 status-accuracy correction as the standard to hold to)

---

## 10. Explicit Non-Goals for Phase 4

- Full Raft/Paxos-grade consensus (a simplified deterministic election is
  sufficient here, per Section 7.2)
- Real geo-IP-based routing (static/hinted region selection only)
- Metrics/tracing pipeline proper (Phase 5) — region health status is
  functional data, not yet wired into a full observability stack
- Vector storage / AI features (Phase 6)
- Auth on the Gateway (Phase 8) — still unauthenticated in dev, same
  known-gap pattern as prior phases

---

## 11. Suggested Build Order (within this phase)

1. Consistency model decision logged (gate — must happen before step 4).
2. Region metadata: latency matrix, region health rollup logic + unit
   tests.
3. Gateway service skeleton + nearest-region routing (Section 5).
4. Region-aware Scheduler extension (Section 6) + unit tests.
5. Cross-region replication wiring (extends Phase 3's replication,
   Section 7.1).
6. Leader election logic (Section 7.2) + unit tests.
7. Full failover flow (Section 8) wired end-to-end.
8. Integration test — all 4 scenarios in Section 9.2. This is the phase's
   real milestone.
9. Dashboard region-health view.
10. Benchmarks + READMEs + decision docs + `PROJECT_STATUS.md` update.

Work through these one at a time. After each numbered step, report status
before moving to the next, per the working agreement in `GEMINI.md`.
