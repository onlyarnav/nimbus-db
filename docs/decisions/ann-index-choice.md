# Architectural Decision Record: ANN Vector Index Choice & Filtering Strategy

## Context
In Phase 6, NimbusDB extends its Rust storage engine (`services/node-agent/src/storage/`) to support vector embeddings, exact search, approximate nearest neighbor (ANN) search, metadata filtering, and hybrid B+Tree predicate + vector similarity queries.

We must select an ANN indexing algorithm and a metadata filtering strategy.

## Decisions

### 1. ANN Index Algorithm: HNSW (Hierarchical Navigable Small World)
NimbusDB will implement an **HNSW** graph index for approximate nearest neighbor search.

#### Rationale
- **Industry Standard**: HNSW is the state-of-the-art graph index used by production vector databases (Qdrant, Cosmos DB Vector Search, Azure AI Search).
- **Resume & Interview Continuity**: Directly connects to the author's previous open-source contribution to Qdrant (duplicate point ID handling): *"Fixed a bug in Qdrant's point-ID handling, then built a native Rust HNSW index inside NimbusDB to demonstrate deep understanding of graph-based vector search."*
- **High Recall & Sub-Millisecond Search**: Provides >95% recall@10 with logarithmic search complexity \(O(\log N)\).

---

### 2. Filtering Strategy: Pre-filtering
NimbusDB will use **Pre-filtering** for metadata-constrained vector queries.

#### Rationale
- **Zero Leakage Guarantee**: Pre-filtering applies metadata equality/predicate constraints (e.g., `region = 'india'`) first using the existing B+Tree or hash index before scoring vector similarity. This guarantees that 100% of returned top-K candidates strictly satisfy the filter condition without returning fewer than K items due to post-filtering discard.
