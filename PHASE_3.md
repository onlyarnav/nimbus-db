# PHASE_3.md — Storage Engine

Companion spec to `GEMINI.md`, `PHASE_1.md`, `PHASE_2.md`. This is the
hardest and highest-value phase in the whole project — per `GEMINI.md`
Section 5, do not compress or shortcut this to move faster. Do not start
until Phase 2's acceptance criteria are fully met.

**Language decision gate:** `GEMINI.md` flags Rust vs C++ as pending. This
must be decided and logged in `docs/decisions/rust-vs-cpp.md` before any
code in this phase is written. This file is written language-agnostically;
translate to your chosen language's idioms once decided.

**Protocol note (revised 2026-07-13):** This phase introduces **no new
external or internal API surface**. The Node Agent's gRPC contract
(`CreateDatabase`, `DeleteDatabase`, `BackupDatabase`, `RestoreDatabase`,
defined in `PHASE_2.md` Section 3.3) stays exactly as-is from the Control
Plane's point of view — this phase only changes what happens *inside* the
Node Agent when those RPCs are called (Section 2 below). `BackupDatabase`/
`RestoreDatabase` move from stub to real implementation here (Section 8),
but the RPC signatures do not change. No REST is introduced anywhere in
this phase — the storage engine has no external clients.

---

## 1. Goal of This Phase

Replace the Phase 2 Node Agent's placeholder storage (a directory
allocation / in-memory map) with a real, crash-consistent, disk-backed
storage engine. This is what turns "distributed system that shuffles
metadata around" into "actual database."

**Definition of done:** a database on a node can survive a hard kill
mid-write and recover correctly via WAL replay; reads/writes go through a
real page-based storage layer with an index; replication between a leader
and follower node is consistent and ACK-gated; all of this has measured
(not estimated) throughput and recovery-time numbers.

---

## 2. Scope Boundary — Read This First

This phase touches **only** `services/node-agent/` internals. The gRPC
contract established in Phase 2 (`CreateDatabase`, `DeleteDatabase`, plus
now-real `BackupDatabase`/`RestoreDatabase`) does not change shape from
the Control Plane's point of view — only what happens *inside* the Node
Agent when those RPCs are called. If the contract needs to change, treat
that as a decision to flag, not a silent break.

---

## 3. Components to Build

```
node-agent/
└── storage/
    ├── page/          ← 4KB page format, page manager
    ├── wal/             ← write-ahead log, crash recovery
    ├── index/             ← hash index (v1), B+Tree (v2)
    ├── compaction/          ← segment merging
    ├── snapshot/               ← periodic checkpoints (also backs real backup/restore)
    └── replication/               ← leader/follower, ACK-based commit
```

Build in this order — each depends on the previous:
**Page Manager → WAL → Crash Recovery → Hash Index → Snapshots →
Compaction → B+Tree → Replication.**

---

## 4. Page Manager

### 4.1 Design
- Fixed 4KB pages. Every read/write to disk happens in page-sized units —
  no partial-page I/O.
- Page header: page ID, page type (data/index/free), checksum, LSN (log
  sequence number — ties the page to the WAL entry that last modified it).
- Free-page tracking: a free list or bitmap so deleted/compacted space can
  be reused rather than leaking disk.

### 4.2 Acceptance
- Unit tests: write a page, read it back, byte-for-byte identical.
- Checksum validation: corrupt a page's bytes on disk directly, confirm
  the page manager detects the checksum mismatch on read rather than
  silently returning corrupt data.

---

## 5. Write-Ahead Log (WAL)

### 5.1 Design
- Every mutation (insert/update/delete) is appended to the WAL **before**
  it's applied to in-memory state or the page store. This ordering is
  non-negotiable — it's the entire point of a WAL.
- WAL entry format: LSN, operation type, key, value (or tombstone for
  delete), checksum.
- WAL is append-only, sequential-write only (no seeking/overwriting
  existing entries) — this is what makes it fast; do not violate this.
- Fsync policy: decide and document — fsync on every write (safest,
  slowest) vs. periodic/batched fsync (faster, small window of loss on
  power failure). Pick one, write the tradeoff down in
  `docs/decisions/wal-fsync-policy.md`.

### 5.2 Flow (must match this exact sequence)
```
Client write
     ↓
Append to WAL (durable before ack)
     ↓
Apply to in-memory state (memtable-style buffer)
     ↓
Acknowledge success to caller
     ↓
(async, later) Flush in-memory state to pages on disk
```

### 5.3 Acceptance
- Unit test: append N entries, read them back in order, confirm exact
  match.
- Unit test: simulate a partial/torn write at the end of the WAL file
  (truncate mid-entry), confirm the recovery reader detects and discards
  the incomplete final entry rather than crashing or corrupting state.

---

## 6. Crash Recovery

### 6.1 Design
- On startup, before serving any request: read the WAL from the last
  known checkpoint (Section 8) forward, replay every entry into memory/
  pages in order.
- Idempotency: replay must be safe to run twice (e.g., if recovery itself
  crashes partway and restarts) — operations should be applied in a way
  that re-applying an already-applied entry doesn't corrupt state (use
  LSN comparison against the page's last-applied LSN to skip already-
  durable entries).

### 6.2 Acceptance (this is the phase's single most important test —
matches the crash-consistency requirement in `GEMINI.md` Section 5)
- **Kill test:** write a known set of records, `SIGKILL` the process at a
  randomly chosen point mid-write-burst (not a graceful shutdown),
  restart, verify:
  - No corruption (page checksums all valid).
  - All writes that were acknowledged to the client before the kill are
    present after recovery.
  - Writes not yet acknowledged may or may not be present — that's
    correct WAL semantics, not a bug; document this explicitly.
- Run this kill test multiple times at different random kill points
  (e.g., 10 runs) to catch timing-dependent bugs — a single successful
  run does not prove correctness.

---

## 7. Index — Hash Index (v1), then B+Tree (v2)

### 7.1 Hash Index (build first)
- In-memory hash map: key → page ID + offset.
- Rebuilt from WAL replay on startup (or from a snapshot + WAL tail, once
  Section 8 exists).
- Acceptance: point lookups by key return correct values; supports
  insert/update/delete.

### 7.2 B+Tree (build second, after Hash Index works end-to-end)
- On-disk B+Tree, node size aligned to the 4KB page size from Section 4.
- Supports range scans (hash index cannot) — this is the reason to build
  it: document this tradeoff explicitly rather than treating B+Tree as a
  strict upgrade.
- Acceptance: point lookups match Hash Index results on the same dataset
  (use this as a correctness cross-check); range queries return correctly
  ordered results.

**Do not skip Hash Index to jump straight to B+Tree** — the simpler
structure de-risks the WAL/page-manager integration before adding tree
balancing complexity on top.

---

## 8. Snapshots

### 8.1 Design
- Periodic checkpoint: flush current in-memory state + page store to a
  consistent on-disk snapshot, record the WAL LSN at the time of the
  snapshot.
- After a snapshot, WAL entries older than the snapshot's LSN can be
  safely truncated/archived (recovery only needs to replay from the last
  snapshot forward).
- Snapshot format must be self-describing enough to support **real
  backup/restore** — this is where the Phase 2 stub gets replaced:
  - `BackupDatabase` (Node Agent gRPC method, contract defined in
    `PHASE_2.md` Section 3.3, stubbed in Phase 2 Section 4.3) now
    triggers a snapshot and returns a reference to it (e.g., snapshot ID +
    storage location) — same RPC signature, real implementation.
  - `RestoreDatabase(id, snapshotRef)` loads a snapshot and replays WAL
    from that point forward, restoring the database to that point in
    time — same RPC signature, real implementation.

### 8.2 Acceptance (closes out the Phase 2 backup/restore deferral)
- **End-to-end backup/restore test:** write data → trigger backup (via
  `BackupDatabase` gRPC call) → simulate data loss (delete the live page
  store) → restore from the backup (via `RestoreDatabase` gRPC call) →
  verify all data present and correct.
- This satisfies the real backup/restore deliverable that was
  intentionally deferred from Phase 2 — update `PROJECT_STATUS.md`'s
  Deferred Items table once this passes.

---

## 9. Compaction

### 9.1 Design
- Over time, updates/deletes leave old page versions and tombstones
  behind (classic LSM-style fragmentation, even in a page-based design).
- Compaction merges live data into fresh pages, discards obsolete
  versions and expired tombstones, returns freed pages to the free list
  (Section 4.1).
- Run compaction as a background process, not blocking reads/writes
  (document any brief locking that is unavoidable).

### 9.2 Acceptance
- Before/after compaction: disk space usage measurably decreases on a
  workload with many updates to the same keys (measured, recorded in
  `docs/benchmarks.md`).
- Data correctness unaffected: read all keys before and after compaction,
  confirm identical results.

---

## 10. Replication

### 10.1 Design
- Leader/follower model. Leader accepts writes, replicates to followers,
  waits for a configurable ACK quorum before reporting commit success to
  the client.
- Flow:
```
Client write → Leader WAL append → Replicate WAL entry to followers
     → Follower(s) append to own WAL, ACK → Leader receives quorum ACKs
     → Leader commits, acknowledges client
```
- Replication traffic between nodes is internal service-to-service
  communication — use gRPC streaming (a natural fit for a continuous WAL
  entry stream), consistent with the internal-RPC decision already
  established in Phase 1/2. Do not introduce a separate protocol here.
- Decide and document ACK quorum policy for this phase (e.g., "ACK from
  all followers" is simplest for now; full quorum/majority logic can be
  refined in Phase 4 alongside multi-region leader election — note this
  as intentional Phase 3 scope, not a gap).

### 10.2 Acceptance
- Integration test: write to leader, confirm follower(s) converge to the
  same state.
- Failure test: kill a follower mid-replication, confirm leader does not
  hang forever (timeout + documented degraded-mode behavior — e.g.,
  proceed with remaining followers, log a warning).
- Measure and record replication lag (time from leader commit to follower
  applying the entry) under a modest write load.

---

## 11. Testing Requirements Summary

### 11.1 Unit Tests (per component — see acceptance sections above)
- Page Manager: read/write correctness, checksum detection.
- WAL: append/replay correctness, torn-write handling.
- Hash Index: CRUD correctness.
- B+Tree: CRUD + range scan correctness, cross-checked against Hash Index.
- Compaction: space reclamation + data correctness pre/post.

### 11.2 Integration Tests (required — these are the phase's acceptance
proof, matching `GEMINI.md` Section 5 requirements exactly)
1. **Crash-consistency test** (Section 6.2) — the non-negotiable one.
2. **Backup/restore end-to-end test** (Section 8.2).
3. **Replication convergence + follower-failure test** (Section 10.2).

### 11.3 What "done" looks like
- [ ] All unit tests pass across every component
- [ ] Crash-consistency kill test passes across ≥10 randomized kill points
- [ ] Backup/restore end-to-end test passes
- [ ] Replication test passes, including follower-failure handling
- [ ] `docs/benchmarks.md` updated with **measured** numbers for: write
      throughput, read throughput, recovery time after crash (by WAL
      size), compaction space reclaimed, replication lag — all from real
      runs, none estimated
- [ ] `docs/decisions/` updated: Rust vs C++ (should already be logged
      before this phase started), WAL fsync policy, ACK quorum policy
- [ ] Node Agent README updated to reflect real storage internals,
      backup/restore no longer listed as a stub
- [ ] `PROJECT_STATUS.md` updated to mark Phase 3 complete, and the
      Deferred Items table updated to close out the backup/restore item

---

## 12. Explicit Non-Goals for Phase 3

- Multi-region routing/leader election beyond a single leader/follower
  set (Phase 4)
- Metrics/tracing pipeline (Phase 5) — `/health` and basic logs only
- Vector storage / AI features (Phase 6)
- Any auth on storage-engine internals (Phase 8) — Node Agent RPCs remain
  trusted-network-only for now, same as Phase 2

---

## 13. Suggested Build Order (within this phase)

1. Language decision logged (gate — must happen before step 2).
2. Page Manager (Section 4) + unit tests.
3. WAL (Section 5) + unit tests.
4. Crash Recovery (Section 6) + the randomized kill test (this is the
   milestone — do not move on until this is solid).
5. Hash Index (Section 7.1) + unit tests, wired into Page Manager/WAL.
6. Snapshots (Section 8) + backup/restore end-to-end test (wired into the
   existing `BackupDatabase`/`RestoreDatabase` gRPC methods from Phase 2)
   — this closes the Phase 2 deferral.
7. Compaction (Section 9) + before/after space measurement.
8. B+Tree (Section 7.2), cross-checked against Hash Index.
9. Replication (Section 10, gRPC streaming) + convergence and
   follower-failure tests.
10. Benchmarks + READMEs + decision docs + `PROJECT_STATUS.md` update.

Work through these one at a time. After each numbered step, report status
before moving to the next, per the working agreement in `GEMINI.md`. Given
this phase's difficulty, expect and budget for each step taking longer
than the equivalent step in Phases 1–2 — do not compress steps 4 or 9 to
hit an arbitrary pace.
