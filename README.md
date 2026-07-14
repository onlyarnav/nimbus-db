# NimbusDB
### A Distributed, AI-Native Cloud Database Platform

NimbusDB is a multi-region, horizontally scalable cloud database platform designed to demonstrate the engineering surface area of Microsoft Azure SQL, Cosmos DB, and Azure Data Factory teams. 

The platform enforces a strict separation between the **Control Plane** (orchestration, scheduling, and metadata tracking) and the **Data Plane** (storage engines, write-ahead logging, and replication), ensuring no shared in-process state.

---

## 1. High-Level Architecture

```
                          ┌─────────────────────┐
                          │   API Gateway        │
                          │  (REST, auth, rate   │
                          │   limiting)           │
                          └──────────┬───────────┘
                                     │
                     ┌───────────────┴───────────────┐
                     │        Control Plane           │
                     │  (Scheduler, Provisioner,       │
                     │   Metadata Service)             │
                     └───────────────┬───────────────┘
                                     │
        ┌────────────────────────────┼────────────────────────────┐
        │                            │                             │
┌───────▼────────┐          ┌────────▼────────┐          ┌─────────▼───────┐
│  Node Agent 1   │          │  Node Agent 2    │          │  Node Agent N     │
│  (Data Plane)   │          │  (Data Plane)    │          │  (Data Plane)     │
│  Storage Engine │          │  Storage Engine  │          │  Storage Engine   │
└─────────────────┘          └──────────────────┘          └───────────────────┘
```

### Component Breakdown
*   **Metadata Service**: The single source of truth for cluster metadata (regions, clusters, active nodes, database layouts). Written in **Go**, uses **PostgreSQL** as the backing store, and communicates internally via **gRPC**.
*   **Capacity Scheduler**: A standalone placement service written in **Go** that evaluates node capacities and schedules databases onto the least-loaded nodes.
*   **Worker Node (Node Agent)**: Simulated client representing compute hosts. Operates a telemetry loop reporting synthetic metrics and hosts a debug HTTP interface for partition testing. *(Storage engine will be implemented in **Rust**).*
*   **Control Plane Dashboard**: A **Next.js** application polling the REST boundary to present real-time cluster health statuses and resource gauges.

---

## 2. Current Build Status: Phase 1 Complete
Phase 1 (Distributed Cluster Foundation) is fully complete. The following features are currently active and tested:
*   **gRPC Internal Routing**: Concurrently hosted gRPC service interfaces (`RegisterNode`, `SendHeartbeat`, `GetNodes`) and external REST boundaries (`GET /health`, `GET /v1/nodes`).
*   **Health Manager Ticker**: Background evaluator checking node heartbeat liveness every 2s to classify nodes (`healthy` / `unhealthy` / `dead` / `overloaded`).
*   **Least Loaded Scheduler**: Placement engine calculating resource scores with exclusions for dead/draining nodes and deprioritization for overloaded instances.
*   **E2E Integration Test Suite**: Complete docker-compose orchestration containing automated chaos injection and state transition assertions.

---

## 3. Repository Directory Layout

```
nimbusdb/
├── GEMINI.md                  (Project Constitution)
├── PROJECT_STATUS.md          (Live Build Status Tracker)
├── docs/
│   ├── benchmarks.md          (Real measured performance latencies)
│   └── decisions/             (Architectural Decision Records)
├── proto/
│   └── metadata_service.proto (Internal gRPC protocol buffer definition)
├── services/
│   ├── metadata-service/      (Metadata registry microservice - Go)
│   ├── scheduler/             (Least Loaded placement scheduler - Go)
│   ├── worker-node/           (Simulated node client agent - Go)
│   └── dashboard/             (Real-time control plane dashboard - Next.js)
├── deploy/
│   └── docker/
│       └── docker-compose.yml (Cluster compose orchestrated stack)
└── tests/
    └── integration/
        └── integration_test.go(Multi-node E2E chaos simulation test runner)
```

---

## 4. Getting Started: How to Run and Validate

### Prerequisites
*   **Go 1.25+**
*   **Node.js 18+ & npm**
*   **Docker & Docker Compose**

---

### Step 1: Run the Automated E2E Integration Test Suite
To spin up the entire cluster topology, inject simulated failures, and verify the scheduler's behavior under crash conditions, execute:

```bash
cd tests/integration
go test -v -timeout 5m
```

This test program will automatically:
1. Spin up the Postgres database, Metadata Service, Scheduler, and 3 Worker Node daemons inside a Docker Compose bridge network.
2. Verify all nodes auto-register and establish active heartbeat logs.
3. Pause `worker-2` heartbeats and assert that the `HealthManager` detects its failure (transitions: `healthy` $\rightarrow$ `unhealthy` at 15s $\rightarrow$ `dead` at 60s).
4. Request a placement from the Scheduler and verify that the dead node is excluded from decisions.
5. Resume `worker-2` heartbeats and verify it recovers successfully to `healthy`.
6. Clean up and delete Docker containers, networks, and test database volumes on exit.

---

### Step 2: Run Local Micro-Benchmarks
To run local performance benchmarks measuring registration and heartbeat database latencies:

```bash
cd services/metadata-service/tests
$env:DATABASE_URL="postgres://postgres:password@localhost:5432/nimbusdb?sslmode=disable"
go test -bench=MetadataService
```

*Note: Requires a local running PostgreSQL instance (or start the container in `deploy/docker` manually first).*

---

### Step 3: Run the Live Dashboard
To visualize the node metrics on a Web UI:

1. Ensure the docker-compose stack is running:
   ```bash
   docker compose -f deploy/docker/docker-compose.yml up -d
   ```
2. Navigate to the dashboard service and start the Next.js server:
   ```bash
   cd services/dashboard
   npm install
   npm run dev
   ```
3. Open [http://localhost:3000](http://localhost:3000) in your browser to view the active cluster health dashboard.

---

## 5. Logged Design Decisions
For details on tradeoffs and selections made, refer to our ADR files under `docs/decisions/`:
*   **Metadata DB**: [metadata-store-choice.md](file:///d:/nimbus-db/docs/decisions/metadata-store-choice.md) (PostgreSQL)
*   **Communication protocol**: [internal-rpc-choice.md](file:///d:/nimbus-db/docs/decisions/internal-rpc-choice.md) (gRPC internally, REST at the dashboard boundary)
*   **Storage engine language**: [rust-vs-cpp.md](file:///d:/nimbus-db/docs/decisions/rust-vs-cpp.md) (Rust)
