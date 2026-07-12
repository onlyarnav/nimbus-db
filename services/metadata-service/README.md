# NimbusDB Metadata Service

The Metadata Service is the central control-plane repository and single source of truth for cluster metadata in NimbusDB. It maintains the registry of regions, clusters, active worker nodes, database schemas, and replicas, and tracks node health status via heartbeats.

## Responsibilities

- Database schema management and migrations.
- Liveness check (`/health` probe).
- *Future*: Registering worker nodes, tracking resource usages, identifying node failures, and selecting candidates for data distribution.

## Tech Stack & Decisions

- **Language**: Go 1.22.12
- **Database**: PostgreSQL (native UUIDs via `gen_random_uuid()`)
- **Internal API**: gRPC (to be built in later steps)
- **Migrations**: managed programmatically using `golang-migrate/migrate/v4` (using embedded SQL files in main, or standard file loader in tests)
- **Database Client**: `github.com/jackc/pgx/v5` connection pool

## Getting Started

### Prerequisites

- Go 1.22+
- Running PostgreSQL database (defaults to `postgres://postgres:postgres@localhost:5432/nimbusdb?sslmode=disable`)

To start a local Postgres database:
```bash
docker compose -f ../../deploy/docker/docker-compose.yml up -d
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection string | `postgres://postgres:postgres@localhost:5432/nimbusdb?sslmode=disable` |
| `PORT` | HTTP Server port | `8080` |

### Running the Service

The service automatically applies all schema migrations on startup.

```bash
DATABASE_URL="postgres://postgres:postgres@localhost:5432/nimbusdb?sslmode=disable" PORT=8080 go run main.go
```

Verify service is running:
```bash
curl http://localhost:8080/health
```

Output:
```json
{"status":"UP","database":"connected"}
```

### Running Tests

To run the unit and integration-level schema tests:
```bash
go test -v ./...
```

---

## Known Gaps (Not Yet Implemented)

As this is the initial skeleton (Phase 1, Step 1), the following features are deliberately **not** built yet:
- **Node Registration**: `POST /v1/nodes/register` (Step 2)
- **Heartbeat Ingestion**: `POST /v1/nodes/{id}/heartbeat` (Step 3)
- **Health Manager**: Detecting dead, slow, or overloaded nodes (Step 4)
- **Scheduler**: Least Loaded placement algorithm (Step 5)
- **Cluster/Region CRUD**: endpoints for clusters and regions management
- **Dashboard APIs**: listing nodes/clusters for Next.js UI consumption (Step 7)
