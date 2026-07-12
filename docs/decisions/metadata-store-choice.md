# Metadata Store Choice: PostgreSQL

## Context and Problem Statement

The Control Plane of NimbusDB requires a metadata store to keep track of cluster topology (regions, clusters, nodes, databases, replicas, and heartbeat history). The database needs to support transaction isolation, relational integrity, unique constraints, and schema migration.

## Decision

We chose **PostgreSQL** as our metadata store.

## Rationale

1. **Experience Alignment**: Reuses existing SQLAlchemy/Alembic experience.
2. **Production Parity**: Matches Microsoft's actual supported engines (Azure SQL, Cosmos DB PostgreSQL), ensuring we build realistic engineering depth.
3. **Dialect Consistency**: Ensures unit tests run against the exact SQL dialect and constraint engine used in production (e.g. native UUID generation via `gen_random_uuid()`).
