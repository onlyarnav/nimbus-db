# Node Agent & Storage Engine (`services/node-agent/`)

The **Node Agent** is NimbusDB's data plane daemon. Built in **Rust** (`edition = "2021"`), it hosts the paged, crash-consistent, indexed, snapshot-capable, and replicating database storage engine.

## Architectural Components

```
services/node-agent/src/storage/
├── page.rs          ← Fixed 4KB pages, PageHeader (ID, type, checksum, LSN), PageManager, free-page tracking
├── wal.rs           ← Append-only Write-Ahead Log (WAL), LogRecord, CRC32 checksums, torn write truncation
├── recovery.rs      ← Crash Recovery engine, LSN idempotency replay, 10-run randomized kill test
├── hash_index.rs    ← In-memory Hash Index for O(1) point lookups
├── btree_index.rs   ← On-disk 4KB page B+Tree Index for ordered range scans
├── snapshot.rs      ← Checkpointing, snapshot file generation, log truncation, backup/restore
├── compaction.rs    ← Background segment merger, tombstone cleanup, free-page recycling
├── replication.rs   ← Leader/Follower streaming WAL log replication, ACK quorum, degraded mode
└── engine.rs        ← Unified StorageEngine orchestrator
```

## gRPC Interface

The Node Agent exposes the `NodeAgent` service defined in `proto/node_agent.proto`:

- `CreateDatabase`: Allocates storage namespace directory and initializes a `StorageEngine` instance.
- `DeleteDatabase`: Deletes database storage namespace and frees disk pages.
- `BackupDatabase`: Triggers a snapshot checkpoint and generates a self-describing `.snap` file reference (**real implementation**).
- `RestoreDatabase`: Loads a `.snap` file, restores page store & indexes, and replays WAL from checkpoint LSN (**real implementation**).

## Running Tests

Run full unit and integration test suite:

```bash
cargo test
```
