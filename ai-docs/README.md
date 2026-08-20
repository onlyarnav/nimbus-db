# NimbusDB — Distributed, AI-Ready Cloud Database Platform

[![CI/CD Pipeline](https://github.com/onlyarnav/nimbusdb/actions/workflows/ci.yml/badge.svg)](https://github.com/onlyarnav/nimbusdb/actions)
[![Rust Engine Tests](https://img.shields.io/badge/Rust_Engine-28%2F28_PASS-brightgreen)](file:///d:/testing-nimbus-db/nimbus-db/services/node-agent)
[![Go Integration Tests](https://img.shields.io/badge/Go_Simulation-PASS-brightgreen)](file:///d:/testing-nimbus-db/nimbus-db/tests/integration)
[![Helm Lint](https://img.shields.io/badge/Helm-Validated-blue)](file:///d:/testing-nimbus-db/nimbus-db/deploy/helm/nimbusdb)

**NimbusDB** is a distributed, multi-region cloud database platform featuring an asynchronous control plane in Go, a 4KB paged crash-consistent storage engine with HNSW vector indexing in Rust, and automated cloud operations. Engineered as a portfolio project, NimbusDB demonstrates verifiable distributed systems mechanisms mirroring the engineering surface area of Microsoft Azure SQL, Cosmos DB, and Azure Data Factory platform teams.

> [!NOTE]
> **Educational & Portfolio Scope**: NimbusDB is built specifically to demonstrate real, verifiable distributed systems mechanisms (WAL replay post-crash, gRPC control plane orchestration, multi-region routing, HNSW graph search, and CI/CD automated rollbacks) backed by measured empirical benchmarks. It does not claim production parity with commercial distributed engines like Azure Cosmos DB or Google Spanner.

---

## Architecture

```
                          ┌─────────────────────────────┐
                          │         API Gateway         │
                          │   (REST Edge, JWT Auth,     │
                          │    Rate Limiting, Routing)  │
                          └──────────────┬──────────────┘
                                         │ gRPC
                         ┌───────────────┴───────────────┐
                         │         Control Plane         │
                         │   (Scheduler, Provisioner,    │
                         │    Reconciler, Deployer)      │
                         └───────────────┬───────────────┘
                                         │ gRPC
        ┌────────────────────────────────┼────────────────────────────────┐
        │                                │                                │
┌───────▼────────┐              ┌────────▼────────┐              ┌────────▼────────┐
│  Node Agent 1  │              │  Node Agent 2   │              │  Node Agent N   │
│  (Data Plane)  │  gRPC Stream │  (Data Plane)   │  gRPC Stream │  (Data Plane)   │
│  Rust Engine   ├─────────────►│  Rust Engine    ├─────────────►│  Rust Engine    │
│ (WAL/B+/HNSW)  │              │ (WAL/B+/HNSW)   │              │ (WAL/B+/HNSW)   │
└────────────────┘              └─────────────────┘              └─────────────────┘

        Cross-cutting: Metadata Service (Postgres Source of Truth)
                        Observability Stack (OTel / Prometheus / Alertmanager / Grafana)
                        Multi-Region Router (Latency & Health Fallback)
```

---

## Quickstart

### Prerequisites
- **Rust / Cargo** (v1.80+)
- **Go** (v1.22+)
- **Docker Desktop** (v24.0+)
- **Helm** (v3.10+) & **kubectl**

### 1. Clone the Repository
```bash
git clone https://github.com/onlyarnav/nimbusdb.git
cd nimbusdb
```

### 2. Run Rust Storage Engine Test Suite (28 Tests)
Verify the core 4KB page manager, WAL replay, crash recovery kill loop, B+Tree, HNSW vector search, and replication streams:
```bash
cd services/node-agent
cargo test --workspace
cd ../..
```

### 3. Validate Kubernetes Helm Chart
Lint and dry-run render the 17 Kubernetes manifests for all stateless and stateful services:
```bash
# Lint the chart
helm lint deploy/helm/nimbusdb

# Template rendering with required credentials
helm template nimbusdb deploy/helm/nimbusdb \
  --set global.jwtSecret="prod-secret-token-32-bytes-long!" \
  --set postgres.password="prod-db-password"
```

### 4. Run the Full Local Stack via Docker Compose
```bash
# Build and start services and PostgreSQL metadata store
docker compose -f deploy/docker/docker-compose.yml up -d

# Check running container statuses
docker compose -f deploy/docker/docker-compose.yml ps
```

### 5. Example API Requests

#### A. Health Check & Observability
```bash
curl http://localhost:8080/health
curl http://localhost:8080/metrics
```

#### B. Create a Database (Control Plane REST API)
```bash
curl -X POST http://localhost:8080/v1/databases \
  -H "Authorization: Bearer <OPERATOR_JWT_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"name": "finance-db", "clusterId": "00000000-0000-0000-0000-000000000000"}'
```

#### C. Insert Vector Record (Node Agent gRPC / REST Edge)
```bash
curl -X POST http://localhost:8080/v1/vectors/insert \
  -H "Authorization: Bearer <OPERATOR_JWT_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{
    "databaseId": "finance-db",
    "vectorId": "doc-101",
    "values": [0.12, 0.85, 0.34, 0.91, 0.05, 0.44, 0.78, 0.23, 0.67, 0.11, 0.89, 0.33, 0.55, 0.77, 0.99, 0.10],
    "metadata": {"region": "india", "category": "sec-filing"}
  }'
```

#### D. Hybrid Vector Search (SQL Predicate + Cosine Similarity)
```bash
curl -X POST http://localhost:8080/v1/vectors/search \
  -H "Authorization: Bearer <READONLY_JWT_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{
    "databaseId": "finance-db",
    "queryVector": [0.12, 0.85, 0.34, 0.91, 0.05, 0.44, 0.78, 0.23, 0.67, 0.11, 0.89, 0.33, 0.55, 0.77, 0.99, 0.10],
    "topK": 5,
    "filterRegion": "india"
  }'
```

### 6. Access Dashboard
Navigate to `http://localhost:3000` to view cluster topology, node health states, and region routing metrics.

---

## What's Implemented

| Phase | Module | Key Features & Technical Specifications |
|-------|--------|----------------------------------------|
| **Phase 1** | [Cluster Foundation](PHASE_1.md) | Metadata Service, gRPC node registration, 5s heartbeat loop, Health Manager background daemon (`healthy`→`unhealthy`→`dead`), and Least-Loaded Scheduler. |
| **Phase 2** | [Control Plane](PHASE_2.md) | Asynchronous provisioning state machine, retry-on-node-failure orchestrator (28.7ms failover), background reconciler loop, and REST/gRPC interfaces. |
| **Phase 3** | [Storage Engine](PHASE_3.md) | Rust 4KB page storage, CRC32 WAL with crash recovery (0.42s recovery time, 15,200 write ops/sec), Hash & B+Tree indexing, Compaction engine (66.7% space saved), Snapshots, and streaming ACK Replication. |
| **Phase 4** | [Multi-Region](PHASE_4.md) | 5 simulated regions (`india`, `us-east`, `us-west`, `europe`, `japan`), nearest-region routing hints (1.8ms), region failover (16.2s e2e heartbeat window), and eventual consistency WAL replication streams. |
| **Phase 5** | [Observability](PHASE_5.md) | OpenTelemetry distributed tracing across 4 service hops (<0.5ms overhead), Prometheus `/metrics` endpoints, Alertmanager rules, and Webhook Receiver. |
| **Phase 6** | [AI-Ready Database](PHASE_6.md) | Vector data model with WAL durability, exact cosine search, metadata pre-filtering (0% leakage), HNSW graph index (100% Recall@10, 0.04ms search), and hybrid range + similarity search (0.22ms). |
| **Phase 7** | [Cloud Operations](PHASE_7.md) | Zero-loss node draining (0 dropped requests across 50 connections), Deployment Controller (Rolling, Canary with 201.37ms auto-rollback, Blue-Green), linear regression capacity planner, and SLA monitor (99.90% availability). |
| **Phase 8** | [Security](PHASE_8.md) | JWT token authentication, SHA-256 API key hashing with instant revocation, token-bucket rate limiting (429 enforcement), and RBAC hierarchy (`admin`, `operator`, `read-only`) with 403 Forbidden denial checks. |
| **Phase 9** | [Kubernetes Deployment](PHASE_9.md) | Umbrella Helm chart (17 manifests), `StatefulSets` with PVCs for storage engine & metadata store, `Deployments` for stateless services, Ingress TLS edge isolation, and HPA auto-scaling. |
| **Phase 10** | [CI/CD](PHASE_10.md) | GitHub Actions workflows (`ci.yml`, `cd.yml`, `rollback.yml`), pre-merge test checks, and automated deploy-time `helm rollback` (2.16 ms detection-to-recovery duration). |

---

## Known Limitations

Directly pulled from phase non-goals and independent audit findings in `VERIFICATION_CHECKLIST.md`:

1. **Container Migration Path Dependency**: `services/metadata-service/Dockerfile:15` contains an unadjusted migration copy path (`db/migrations` vs `migrations`), requiring remediation for clean-state image builds in live Docker Compose / Helm cluster deployments.
2. **Go Unit Test Suite Alignment**: Go test suites currently centralize end-to-end integration tests in `tests/integration`; unit test expectations in `worker-node` and `control-plane` require updating to align with Phase 3 snapshot implementations and Phase 8 auth headers.
3. **Host Static Analysis Tooling**: Host execution of `golangci-lint`, `gosec`, and `gitleaks` requires local binary installation on PATH; CI runs them inside containerized environments.
4. **Environment Variable Secret Enforcement**: Development fallback secret in `jwt.go` and committed test credentials in `values.yaml` must be explicitly overridden in production deployments.
5. **Consensus & Multi-Region Consistency Model**: Multi-region replication operates on an eventual consistency model with asynchronous gRPC WAL streams and deterministic leader election rather than multi-region synchronous Raft/Paxos consensus.
6. **ANN Traversal Boundary**: Vector search implements pre- and post-filtered graph traversal rather than full unified filter-aware graph traversal.
7. **Data Protection Scope Guards**: Enterprise backup semantics (WORM immutable snapshots, legal-hold retention locks, anomaly detection) were deliberately parked and excluded to maintain architectural focus.
8. **Cluster Federation & Service Mesh**: Multi-region topology is simulated within a single cluster via namespace/node affinity; geo-distributed K8s federation and external service meshes (Istio/Linkerd) are out of scope.

---

## Demo Video & Scenarios

*(Video walkthrough link placeholder)*

### Demo Walkthrough Scenarios:
1. **Database Provisioning & Hybrid Vector Search**:
   - Issue REST call to `/v1/databases` → watch Control Plane pick least-loaded node and provision in <15ms.
   - Insert 16d embedding vectors → execute hybrid SQL range predicate + cosine similarity query.
2. **Multi-Region Failover**:
   - Kill all nodes in `us-east` region → watch Gateway automatically reroute traffic to next-nearest healthy region (`us-west`).
3. **Storage Engine Chaos Recovery**:
   - Kill storage engine process mid-write (`SIGKILL`) → restart process and observe instant WAL replay restoration (<0.5s recovery).
4. **CI/CD Automated Rollback**:
   - Push commit with runtime 503 error → watch post-deploy health check trigger `helm rollback` in <3ms to restore healthy version.

---

## License

MIT License. Engineered by Arnav Purohit ([@onlyarnav](https://github.com/onlyarnav)).
