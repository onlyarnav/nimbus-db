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
├── vector/          ← Vector storage engine extension
│   ├── distance.rs  ← Cosine similarity metric calculation
│   ├── record.rs    ← VectorRecord data model (id, data, embedding, metadata)
│   ├── filter.rs    ← Metadata predicate filtering (pre-filtering)
│   ├── hnsw.rs      ← HNSW graph vector index for approximate nearest neighbor (ANN) search
│   └── vector_test.rs ← Vector unit, correctness, recall benchmark & crash recovery tests
└── engine.rs        ← Unified StorageEngine orchestrator
```

## gRPC Interface

The Node Agent exposes the `NodeAgent` service defined in `proto/node_agent.proto`:

- `CreateDatabase`: Allocates storage namespace directory and initializes a `StorageEngine` instance.
- `DeleteDatabase`: Deletes database storage namespace and frees disk pages.
- `BackupDatabase`: Triggers a snapshot checkpoint and generates a self-describing `.snap` file reference (**real implementation**).
- `RestoreDatabase`: Loads a `.snap` file, restores page store & indexes, and replays WAL from checkpoint LSN (**real implementation**).
- `InsertVector`: Stores vector embedding with metadata into storage engine, appending to WAL first for **durability**.
- `SearchVector`: Performs exact cosine similarity or HNSW ANN graph search with metadata pre-filtering.

## Embedding Model Note (Dev/Demo Tooling)

*Note on Embedding Generation:* NimbusDB does **not** train or host embedding models natively. Pretrained embedding models (e.g. `sentence-transformers` via Python scripts or external APIs) are used purely as dev/demo tooling for generating test-dataset vectors. The NimbusDB storage engine itself is **embedding-model-agnostic** and stores/indexes whatever fixed-dimension floating-point vectors it receives.

## Running Tests

Run full unit, integration, recall benchmark, and crash recovery test suite:

```bash
cargo test
```
