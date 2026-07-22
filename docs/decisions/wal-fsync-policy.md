# Architectural Decision Record: Write-Ahead Log (WAL) Fsync Policy

## Context
The storage engine's durability guarantees depend on when Write-Ahead Log (WAL) record mutations are flushed (`fsync`'d) to persistent physical disk. We need to choose between flushing on every write operation versus batched/periodic flushing.

## Decision
NimbusDB's default WAL fsync policy is **SyncPerWrite** for strict durability guarantees, with an optional **BatchedSync** mode (interval or buffer size threshold) for benchmark performance testing.

## Rationale
1. **Zero Data Loss Guarantee**: Flushing on every acknowledged write guarantees that any operation reported as successful to a client will survive power failure or hard process crash (`SIGKILL`).
2. **Crash Recovery Reliability**: A synchronous WAL write path simplifies recovery replay by eliminating ambiguous partially-flushed buffer states.
3. **Configurability**: Exposing a batched fsync option allows performance benchmarking under high concurrency while explicitly documenting the tradeoff (up to N ms or K bytes of data loss on hard system crashes).

## Implications
- WAL engine accepts a `SyncPolicy` configuration (`SyncPerWrite` vs `Batched`).
- The default in production is `SyncPerWrite`.
