# PHASE_2.md — Control Plane

Companion spec to `GEMINI.md` and `PHASE_1.md`. Do not start this phase
until Phase 1's acceptance criteria are fully met (see `PROJECT_STATUS.md`).
This phase assumes a working Metadata Service, node registration,
heartbeats, Health Manager, and Scheduler from Phase 1 — it builds on top
of them, it does not rebuild them.

---

## 1. Goal of This Phase

Turn the cluster foundation from Phase 1 into something that can actually
**provision a database** end-to-end: receive a request, pick a node via
the existing scheduler, tell that node to create a database, record the
result in metadata, and handle the case where the chosen node fails
mid-provision.

**Definition of done:** `POST /createDatabase` reliably provisions a
database on a healthy node; killing the chosen node mid-provision causes
an automatic retry onto a different node; metadata always reflects ground
truth (no databases pointing at dead nodes, no orphaned records).

---

## 2. Services Touched in This Phase

```
services/
├── control-plane/     ← NEW: orchestrates the createDatabase flow (Go)
├── node-agent/          ← NEW: runs on each worker, exposes DB lifecycle ops (Go)
├── metadata-service/     ← EXTENDED: databases/replicas tables become live
└── scheduler/              ← REUSED as-is from Phase 1, no changes expected
```

`control-plane` is a new service, not a rename of the scheduler — keep the
control-plane/data-plane separation from `GEMINI.md` Section 3 explicit in
the code layout, not just conceptually.

---

## 3. Control Plane

### 3.1 Responsibilities
- Owns the `POST /createDatabase` (and `DELETE /database/{id}`) flow.
- Talks to Scheduler (Phase 1) to pick a node.
- Talks to Node Agent (this phase, Section 4) to actually create the
  database on that node.
- Talks to Metadata Service to persist the outcome.
- Owns retry logic when a chosen node fails mid-provision.

### 3.2 API Surface

| Method | Path                      | Purpose                                  |
|--------|----------------------------|--------------------------------------------|
| POST   | `/v1/databases`            | Create a database (the core flow)         |
| GET    | `/v1/databases/{id}`       | Get database status/location              |
| GET    | `/v1/databases`            | List all databases                        |
| DELETE | `/v1/databases/{id}`       | Delete a database                         |
| GET    | `/health`                   | Service's own liveness probe               |

**Create request/response:**
```json
// POST /v1/databases
{ "name": "orders-db", "clusterId": "..." }

// 202 response (async — provisioning takes time)
{ "databaseId": "...", "status": "provisioning" }

// GET /v1/databases/{id} once complete
{
  "databaseId": "...",
  "name": "orders-db",
  "status": "active",       // provisioning | active | failed | deleting
  "nodeId": "...",
  "attempts": 1,
  "createdAt": "..."
}
```

### 3.3 The Create Flow (must match this exact sequence — it's the phase's
core deliverable, per the original spec)

```
Receive request
     ↓
Validate (name non-empty, cluster exists, cluster has ≥1 healthy node)
     ↓
Choose cluster (from request; validate it exists)
     ↓
Choose node (call Scheduler's placement API from Phase 1)
     ↓
Provision (call chosen Node Agent's Create Database RPC)
     ↓
   ├─ success → Update metadata (status=active, nodeId=chosen) → Return success
   └─ failure/timeout → Mark node as suspect, retry: choose next node,
                          provision again (max 3 attempts total)
                          → if all attempts exhausted: status=failed,
                            update metadata, return error
```

Implement this as an explicit state machine or a clearly sequential
function — do not scatter retry logic across callbacks in a way that's
hard to trace. This flow is what gets whiteboarded in interviews; it needs
to be legible in the code.

### 3.4 Timeout & Retry Parameters
Pick concrete values and write them down (these are starting defaults, not
benchmarked — same caveat as Phase 1's health thresholds):
- Provision call timeout: 10s per attempt
- Max attempts: 3
- Backoff between attempts: none required for Phase 2 (nodes are either up
  or down in this simulated environment — no need for exponential backoff
  complexity yet; note this as a known simplification)

---

## 4. Node Agent

### 4.1 Responsibilities
Runs as a sidecar/companion process alongside each Worker Node from
Phase 1 (same process is acceptable — extend the Phase 1 worker rather
than standing up a fully separate binary, unless that gets awkward).

Exposes a small RPC surface that the Control Plane calls:

| RPC / Endpoint          | Purpose                                          |
|---------------------------|----------------------------------------------------|
| `CreateDatabase(name)`     | Allocates storage, registers the DB locally         |
| `DeleteDatabase(id)`       | Removes a database from this node                    |
| `BackupDatabase(id)`        | **Stub only in this phase** — see Section 4.3        |
| `RestoreDatabase(id, snap)` | **Stub only in this phase** — see Section 4.3        |

### 4.2 What "Create Database" Actually Does in Phase 2
There is no real storage engine yet (that's Phase 3). So `CreateDatabase`
in this phase:
- Allocates a directory/namespace for the database on local disk (or an
  in-memory map, if disk allocation feels premature — pick one and note
  it).
- Registers an endpoint (host:port or path) that a future client could
  connect to.
- Returns success/failure realistically (including simulated random
  failure injection for testing retries — see Section 6).

Do not build real data storage here. This is intentionally a thin
placeholder that Phase 3 will slot a real storage engine underneath
without changing this RPC's contract.

### 4.3 Backup/Restore — Stay Stubs in This Phase (explicit decision)
Backup/restore are **intentionally left as stubs** (return
`not_implemented` or a fixed mock response) in Phase 2. Real snapshot-based
backup requires the WAL/snapshot mechanism from Phase 3 to exist first —
building it now would mean throwing it away and rebuilding it properly
once Phase 3 lands. Revisit as a **Phase 3 extension** once storage engine
snapshots exist; do not treat this as final scope, just correct
sequencing.

---

## 5. Metadata Synchronization

- The Control Plane must **never** hold placement decisions in its own
  memory as the source of truth — every state transition (`provisioning` →
  `active`/`failed`) is written to the Metadata Service immediately.
- If the Control Plane crashes mid-provision, on restart it must be able
  to reconcile: query Metadata Service for any database stuck in
  `provisioning` past a reasonable timeout, and either retry or mark
  failed. (A simple reconciliation loop polling every N seconds is enough
  for this phase — no need for anything fancier.)
- Metadata Service `databases` and `replicas` tables (created as
  placeholders in Phase 1) become live in this phase — `databases.node_id`
  is now actually populated and meaningful.

---

## 6. Failure Injection (required for testing retries)

The Node Agent needs a way to simulate provisioning failure so the retry
path in Section 3.3 is actually exercised, not just theorized:
- A debug flag/endpoint on the Node Agent to force the next N
  `CreateDatabase` calls to fail or hang past timeout.
- This is a test-only mechanism — document clearly in the Node Agent's
  README that it must never be enabled outside test environments.

---

## 7. Testing Requirements

### 7.1 Unit Tests
- Control Plane: create-flow validation logic (empty name rejected,
  nonexistent cluster rejected, cluster with zero healthy nodes rejected).
- Control Plane: retry counter logic — given N simulated failures, confirm
  it retries exactly up to max attempts and then reports failure, not an
  infinite loop.
- Node Agent: CreateDatabase allocates a unique namespace per call, rejects
  duplicate database names on the same node.

### 7.2 Integration Test (required — this is the phase's acceptance proof)
Using the Phase 1 integration test setup (Metadata Service + 3 worker
nodes) plus the new Control Plane and Node Agents:

1. **Happy path:** `POST /v1/databases` with a valid request → assert
   response eventually shows `status: active` with a `nodeId` pointing at
   a real, healthy node.
2. **Retry path:** enable failure injection on the node the Scheduler is
   expected to pick first → `POST /v1/databases` → assert it retries onto
   a different node and still ends in `status: active`. This directly
   satisfies the "retries onto Node B" acceptance criterion from
   `GEMINI.md`.
3. **Exhausted retries:** enable failure injection on all nodes → assert
   `status: failed` after exactly `max attempts` tries, with the failure
   reason recorded in metadata (not silently dropped).
4. **Crash-recovery reconciliation:** kill the Control Plane mid-provision
   (before metadata update) → restart it → assert the reconciliation loop
   (Section 5) resolves the stuck `provisioning` record instead of leaving
   it stuck forever.

### 7.3 What "done" looks like
- [ ] All unit tests pass
- [ ] Integration test (all 4 scenarios in 7.2) passes reliably (run 3x,
      fix or document flakiness)
- [ ] `docs/benchmarks.md` updated with: end-to-end provision latency
      (happy path), retry-path latency, all measured from actual test runs
- [ ] Node Agent and Control Plane each have a README (what it does, how
      to run it, known gaps — explicitly list backup/restore as stubbed
      pending Phase 3)
- [ ] `docs/decisions/` updated with: RPC framework choice reused/confirmed
      from Phase 1, timeout/retry constants and rationale
- [ ] `PROJECT_STATUS.md` updated to mark Phase 2 complete with a summary

---

## 8. Explicit Non-Goals for Phase 2

- Real storage engine, WAL, pages, indexes (Phase 3)
- Real backup/restore (deferred to a Phase 3 extension — see Section 4.3)
- Multi-region-aware scheduling (Phase 4)
- Metrics/tracing beyond `/health` (Phase 5)
- Auth on any endpoint (Phase 8) — these APIs remain unauthenticated in
  dev for now, note this clearly as a known gap, not an oversight

---

## 9. Suggested Build Order (within this phase)

1. Extend Metadata Service: activate `databases`/`replicas` tables with
   real read/write paths (they were schema-only placeholders in Phase 1).
2. Node Agent: `CreateDatabase`/`DeleteDatabase` (real logic per Section
   4.2), `BackupDatabase`/`RestoreDatabase` (stubs per Section 4.3), plus
   the failure-injection debug hook (Section 6).
3. Control Plane: validation logic, Scheduler integration (reuse Phase 1
   API, no changes there), Node Agent RPC client.
4. Control Plane: retry logic + state machine (Section 3.3).
5. Control Plane: reconciliation loop for crash recovery (Section 5).
6. Integration test — all 4 scenarios in Section 7.2. This is the phase's
   real milestone.
7. Benchmarks + READMEs + decision docs + `PROJECT_STATUS.md` update —
   close out the phase.

Work through these one at a time. After each numbered step, report status
before moving to the next, per the working agreement in `GEMINI.md`.
