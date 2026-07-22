# Architectural Decision Record: Replication ACK Quorum Policy

## Context
In Phase 3's leader/follower replication model, writes appended to the leader's Write-Ahead Log (WAL) are streamed to follower nodes. We need to establish an ACK quorum policy determining when the leader considers a write transaction committed.

## Decision
For Phase 3, we adopt an **All-Followers ACK (Sync Replication)** default policy: the leader waits for ACKs from all active registered followers before returning success to the caller. A configurable deadline timeout (default 3s) ensures that if a follower fails or crashes, the leader switches to **Degraded Mode** (logging a warning and proceeding with remaining healthy followers) rather than stalling writes indefinitely.

## Rationale
1. **Strong Consistency Baseline**: Waiting for all healthy followers guarantees strict data consistency across nodes prior to Phase 4 multi-region election.
2. **Crash Resilience**: Timeout-gated degraded mode prevents a single dead follower node from blocking cluster writes.
3. **Phase 4 Preparation**: Quorum definitions will expand to majority/raft quorum models in Phase 4.

## Implications
- Replication engine tracks active follower stream channels and ACKs per LSN.
- Dead/unresponsive followers trigger degraded mode after deadline expiration.
