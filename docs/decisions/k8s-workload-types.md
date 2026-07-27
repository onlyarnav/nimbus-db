# Architectural Decision Record: Kubernetes Workload Type Classification (`StatefulSet` vs `Deployment`)

## Context
When packaging NimbusDB for Kubernetes deployment, each service must be classified as either stateful or stateless to determine its Kubernetes workload primitive (`StatefulSet` with `PersistentVolumeClaim` vs stateless `Deployment`).

## Decision & Workload Matrix

### 1. Stateful Workloads (`StatefulSet` + `PVC`)
- **Metadata Postgres (`metadata-postgres`)**:
  - *Rationale*: Persistent relational store for clusters, nodes, databases, regions, and heartbeat state. Requires stable identity and persistent storage (`/var/lib/postgresql/data`).
- **Worker Node Agent (`worker-node`)**:
  - *Rationale*: Hosts the Phase 3 storage engine data (4KB pages, B+Tree indexes, WAL log files, vector graph indexes). Requires stable ordinal pod network identities and persistent volume attachment per replica. Treating the storage engine as stateless would cause permanent data loss upon pod restarts.

### 2. Stateless Workloads (`Deployment`)
- `gateway`: Stateless REST edge routing requests to internal gRPC endpoints.
- `auth-service`: Stateless JWT token signing, verification, and API key hashing service.
- `scheduler`: Stateless placement decision engine reading cluster stats from Metadata Service.
- `control-plane`: Stateless database creation state machine and reconciler.
- `deployment-controller`: Stateless deployment orchestrator.
- `capacity-planner`: Stateless forecasting engine calculating linear regression trends.
- `sla-monitor`: Stateless SLO aggregation service.
- `dashboard`: Next.js web application frontend.
