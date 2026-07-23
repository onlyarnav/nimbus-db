# Architectural Decision Record: Multi-Region Consistency Model

## Context
In Phase 4, NimbusDB expands from single-region operation to a geo-distributed multi-region deployment across 5 simulated regions (`india`, `us-east`, `us-west`, `europe`, `japan`). We must decide between an **Eventual Consistency** model (with bounded staleness) versus a **Strong Consistency** model (requiring synchronous cross-region commit coordination).

## Decision
NimbusDB will default to **Eventual Consistency with Bounded Staleness** for multi-region replication.
- Local writes to the primary/leader region's storage engine are committed instantly after local WAL write and local follower ACK.
- Asynchronous gRPC WAL streaming replicates writes across regional boundaries.
- Follower regions expose read endpoints with a measurable, bounded staleness window (e.g. maximum $N$ LSN entries or $M$ milliseconds lag).

## Rationale
1. **Latency Alignment with Target Infrastructure**: Synchronous cross-region coordination (Strong Consistency) incurs massive network Round Trip Times (RTT) on every write (e.g. ~180ms RTT between India and US East). Eventual consistency maintains low write latency (< 15ms) in the primary region.
2. **Cosmos DB Architectural Relevance**: Microsoft Azure Cosmos DB defaults to session / eventual / bounded-staleness consistency models across multi-region write accounts to balance SLA availability with geo-replication throughput. This aligns directly with NimbusDB's Azure Data Engineering targeting.
3. **Measurable Staleness Metric**: Eventual consistency allows measuring and reporting real replication lag (in ms and LSN gap) in `docs/benchmarks.md`.

## Implications
- Reads served directly from follower regions may briefly lag behind the primary region by a bounded window.
- Failover of a primary region to a follower region promotes the follower with the highest LSN to minimize data loss.
