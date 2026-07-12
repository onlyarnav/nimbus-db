# Metadata Store Choice: PostgreSQL

## Context and Problem Statement

The Control Plane of NimbusDB requires a metadata store to keep track of cluster topology (regions, clusters, nodes, databases, replicas, and heartbeat history). The database needs to support transaction isolation, relational integrity, unique constraints, and schema migration.

We evaluated two options:
1. **SQLite**: Good for lightweight, zero-configuration local development.
2. **PostgreSQL**: Strong relational consistency, native UUID generation mechanisms, and matches production deployment targets.

## Decision

We chose **PostgreSQL** for both local development and production.

## Rationale

1. **SQL Dialect Consistency**: SQLite and PostgreSQL differ in native DDL features (e.g. UUID generation with `gen_random_uuid()`, `TIMESTAMPTZ`, constraints, altering tables, etc.). Using PostgreSQL locally prevents writing SQLite-specific fallback code or maintaining dual migrations.
2. **Feature Alignment**: Native support for UUIDs and unique multi-column constraints, matching the specs in `PHASE_1.md`.
3. **Operational Experience**: Standardizing on Postgres leverages existing experience with SQL tooling and ensures unit tests run against the exact dialect used in production.
