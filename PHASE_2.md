# PHASE_2.md — Control Plane

Companion spec to `GEMINI.md` and `PHASE_1.md`. Do not start this phase
until Phase 1's acceptance criteria are fully met (see `PROJECT_STATUS.md`).
This phase assumes a working Metadata Service, node registration,
heartbeats, Health Manager, and Scheduler from Phase 1 — it builds on top
of them, it does not rebuild them.

**Protocol note (revised 2026-07-13):** Per `docs/decisions/internal-rpc-choice.md`,
all service-to-service calls in this phase (Control Plane ↔ Scheduler,
Control Plane ↔ Metadata Service, Control Plane ↔ Node Agent) are **gRPC**,
not REST. Only the client/dashboard-facing edge (`/v1/databases` and
friends, called by external clients or the dashboard) is REST/JSON. This
mirrors the fix already applied to Phase 1 — do not repeat the earlier
drift where internal APIs were built as REST.

---

## 1. Goal of This Phase

Turn the cluster foundation from Phase 1 into something that can actually
**provision a database** end-to-end: receive a request, pick a node via
the existing scheduler, tell that node to create a database, record the
result in metadata, and handle the case where the chosen node fails
mid-provision.

**Definition of done:** `POST /v1/databases` reliably provisions a
database on a healthy node; killing the chosen node mid-provision causes
an automatic retry onto a different node; metadata always reflects ground
truth (no databases pointing at dead nodes, no orphaned records).

---

## 2. Services Touched in This Phase

```
services/
├── control-plane/     ← NEW: orchestrates the createDatabase flow (Go)
├── node-agent/          ← NEW: runs on each worker, exposes DB lifecycle ops via gRPC (Go)
├── metadata-service/     ← EXTENDED: databases/replicas tables become live
└── scheduler/              ← REUSED as-is from Phase 1, no changes expected
```

`control-plane` is a new service, not a rename of the scheduler — keep the
control-plane/data-plane separation from `GEMINI.md` Section 3 explicit in
the code layout, not just conceptually.

---

## 3. Control Plane

### 3.1 Responsibilities
- Owns the client-facing create/delete database flow (REST edge).
- Talks to Scheduler (Phase 1) via **gRPC** to pick a node.
- Talks to Node Agent (this phase, Section 4) via **gRPC** to actually
  create the database on that node.
- Talks to Metadata Service via **gRPC** to persist the outcome.
- Owns retry logic when a chosen node fails mid-provision.

### 3.2 External (client-facing) API — REST, unchanged in shape

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

### 3.3 Internal API — gRPC (this is what actually changed)

Define in `proto/control_plane.proto` (or reuse/extend `proto/scheduler.proto`
and `proto/node_agent.proto` from Phase 1 where applicable):

```protobuf
// Control Plane → Scheduler (Scheduler service defined in Phase 1,
// reused here — do not redefine, just consume it)
rpc ScheduleDatabase(ScheduleRequest) returns (ScheduleResponse);

message ScheduleRequest {
  string cluster_id = 1;
}
message ScheduleResponse {
  string node_id = 2;
  double score = 3;
}

// Control Plane → Node Agent (new in this phase)
service NodeAgent {
  rpc CreateDatabase(CreateDatabaseRequest) returns (CreateDatabaseResponse);
  rpc DeleteDatabase(DeleteDatabaseRequest) returns (DeleteDatabaseResponse);
  rpc BackupDatabase(BackupDatabaseRequest) returns (BackupDatabaseResponse);   // stub, see 4.3
  rpc RestoreDatabase(RestoreDatabaseRequest) returns (RestoreDatabaseResponse); // stub, see 4.3
}

message CreateDatabaseRequest {
  string name = 1;
  string database_id = 2;
}
message CreateDatabaseResponse {
  bool success = 1;
  string endpoint = 2;
  string error = 3;
}

// Control Plane → Metadata Service (extends Phase 1's metadata gRPC
// service with database/replica write paths)
rpc UpdateDatabaseStatus(UpdateDatabaseStatusRequest) returns (UpdateDatabaseStatusResponse);
```

Keep `.proto` files under a shared `proto/` directory at repo root so all
Go services generate from the same source of truth — do not let each
service maintain its own divergent copy.

### 3.4 The Create Flow (must match this exact sequence — it's the phase's
core deliverable, per the original spec)

```
Receive request (REST, client-facing)
     ↓
Validate (name non-empty, cluster exists, cluster has ≥1 healthy node)
     ↓
Choose cluster (from request; validate it exists)
     ↓
Choose node (gRPC call: Scheduler.ScheduleDatabase)
     ↓
Provision (gRPC call: NodeAgent.CreateDatabase on chosen node)
     ↓
   ├─ success → Update metadata (gRPC: MetadataService.UpdateDatabaseStatus,
   │              status=active, nodeId=chosen) → Return success (REST)
   └─ failure/timeout → Mark node as suspect, retry: choose next node,
                          provision again (max 3 attempts total)
                          → if all attempts exhausted: status=failed,
                            update metadata, return error (REST)
```

Implement this as an explicit state machine or a clearly sequential
function — do not scatter retry logic across callbacks in a way that's
hard to trace. This flow is what gets whiteboarded in interviews; it needs
to be legible in the code, and the REST-in/gRPC-out boundary should be
obvious at a glance (e.g., a thin HTTP handler that immediately calls into
a gRPC-calling orchestrator function — don't mix HTTP and gRPC concerns in
the same function).

### 3.5 Timeout & Retry Parameters
Pick concrete values and write them down (these are starting defaults, not
benchmarked — same caveat as Phase 1's health thresholds):
- Provision gRPC call timeout: 10s per attempt (use gRPC's built-in
  deadline propagation via `context.WithTimeout`, not a manual timer)
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

Exposes a **gRPC** service (`NodeAgent`, Section 3.3) that the Control
Plane calls — no REST anywhere in this service, it has no external
clients.

| RPC                          | Purpose                                          |
|---------------------------|----------------------------------------------------|
| `CreateDatabase`     | Allocates storage, registers the DB locally         |
| `DeleteDatabase`       | Removes a database from this node                    |
| `BackupDatabase`        | **Stub only in this phase** — see Section 4.3        |
| `RestoreDatabase` | **Stub only in this phase** — see Section 4.3        |

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
Backup/restore are **intentionally left as stubs** (return an
`UNIMPLEMENTED` gRPC status or a fixed mock response) in Phase 2. Real
snapshot-based backup requires the WAL/snapshot mechanism from Phase 3 to
exist first — building it now would mean throwing it away and rebuilding
it properly once Phase 3 lands. Revisit as a **Phase 3 extension** (see
`PHASE_3.md` Section 8) once storage engine snapshots exist; do not treat
this as final scope, just correct sequencing.

---

## 5. Metadata Synchronization

- The Control Plane must **never** hold placement decisions in its own
  memory as the source of truth — every state transition (`provisioning` →
  `active`/`failed`) is written to the Metadata Service (via gRPC)
  immediately.
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
path in Section 3.4 is actually exercised, not just theorized:
- A debug gRPC method or config flag on the Node Agent to force the next N
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
- gRPC layer: use in-process gRPC test servers (bufconn in Go) for unit
  tests rather than spinning up real network listeners — faster and more
  reliable.

### 7.2 Integration Test (required — this is the phase's acceptance proof)
Using the Phase 1 integration test setup (Metadata Service + 3 worker
nodes, all communicating over gRPC) plus the new Control Plane and Node
Agents:

1. **Happy path:** `POST /v1/databases` (REST, client-facing) with a valid
   request → assert response eventually shows `status: active` with a
   `nodeId` pointing at a real, healthy node.
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
- [ ] `docs/decisions/` confirms gRPC usage here is consistent with
      `internal-rpc-choice.md` from Phase 1 — no new decision needed,
      just confirm no drift occurred
- [ ] `PROJECT_STATUS.md` updated to mark Phase 2 complete with a summary

---

## 8. Explicit Non-Goals for Phase 2

- Real storage engine, WAL, pages, indexes (Phase 3)
- Real backup/restore (deferred to a Phase 3 extension — see Section 4.3)
- Multi-region-aware scheduling (Phase 4)
- Metrics/tracing beyond `/health` (Phase 5)
- Auth on any endpoint (Phase 8) — these APIs (both REST edge and internal
  gRPC) remain unauthenticated in dev for now, note this clearly as a
  known gap, not an oversight

---

## 9. Suggested Build Order (within this phase)

1. Extend Metadata Service: activate `databases`/`replicas` tables with
   real read/write paths (they were schema-only placeholders in Phase 1),
   exposed via new gRPC RPCs (`UpdateDatabaseStatus`, etc.).
2. Define `proto/node_agent.proto` and generate stubs.
3. Node Agent: `CreateDatabase`/`DeleteDatabase` (real logic per Section
   4.2), `BackupDatabase`/`RestoreDatabase` (stubs per Section 4.3), plus
   the failure-injection debug hook (Section 6) — all gRPC.
4. Control Plane: REST handler layer (client-facing), validation logic,
   Scheduler gRPC client (reuse Phase 1 proto, no changes there), Node
   Agent gRPC client.
5. Control Plane: retry logic + state machine (Section 3.4).
6. Control Plane: reconciliation loop for crash recovery (Section 5).
7. Integration test — all 4 scenarios in Section 7.2. This is the phase's
   real milestone.
8. Benchmarks + READMEs + `PROJECT_STATUS.md` update — close out the
   phase.

Work through these one at a time. After each numbered step, report status
before moving to the next, per the working agreement in `GEMINI.md`.
