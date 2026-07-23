# Architectural Decision Record: Multi-Region Leader Election Approach

## Context
When a primary database region experiences a hard outage (all nodes down), NimbusDB must perform failover by electing a new primary/leader region from the healthy follower regions. We need to select an appropriate leader election strategy.

## Decision
NimbusDB uses a **Deterministic Highest-LSN / Priority Leader Election** strategy managed by the Control Plane & Metadata Service.
- Upon detecting primary region failure (Region Health = `down`), the Health Manager / Reconciler triggers election among candidate follower regions.
- Candidate regions are evaluated based on:
  1. **Replication Catch-Up Level (LSN)**: Region with the highest applied WAL LSN is prioritized to minimize data loss.
  2. **Deterministic Priority Tie-Breaker**: Static region priority order (`india` > `us-east` > `us-west` > `europe` > `japan`).
- The promoted region is marked as `leader` in Metadata Service, and cross-region replication stream sources are updated.

## Rationale
1. **Scope Boundary**: A full Raft/Paxos consensus implementation across regions is out of scope for Phase 4 (per `PHASE_4.md` Section 7.2) and adds unnecessary consensus protocol complexity without altering the failover state machine.
2. **Determinism**: Deterministic priority and LSN comparison eliminates election split-brain or tie-breaker flakiness during test runs.
3. **Control Plane Governance**: Matches Azure SQL Auto-Failover Groups where the control plane orchestrates primary region promotion based on telemetry and catch-up status.

## Implications
- Failover window is bounded by node failure detection time + region health rollup + metadata promotion write (~1-2s total).
- Rejoining failed regions re-enter cluster in `follower` role to prevent split-brain leadership.
