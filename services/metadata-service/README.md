# NimbusDB Metadata Service

The Metadata Service is the central control-plane repository and single source of truth for cluster metadata in NimbusDB. It maintains the registry of regions, clusters, active worker nodes, database schemas, and replicas, and tracks node health status via heartbeats.

## Responsibilities

- Database schema management and migrations.
- Liveness check (`/health` probe).
- Worker node registration (`RegisterNode` gRPC API).
- Periodic heartbeat processing (`SendHeartbeat` gRPC API).
- Cluster health and state classification (`HealthManager` background daemon checking every 2 seconds).
- Exposing topology listings (`GetNodes` gRPC and REST `/v1/nodes` API).

## Tech Stack & Decisions

- **Language**: Go 1.25.0
- **Database**: PostgreSQL (native UUIDs via `gen_random_uuid()`)
- **Internal API**: gRPC (listening on port `50051`)
- **Dashboard API**: REST/JSON (listening on port `8080`)
- **Migrations**: managed programmatically using `golang-migrate/migrate/v4` (using embedded SQL files in main, or standard file loader in tests)
- **Database Client**: `github.com/jackc/pgx/v5` connection pool

## Getting Started

### Prerequisites

- Go 1.25+
- Running PostgreSQL database (defaults to `postgres://postgres:password@localhost:5432/nimbusdb?sslmode=disable`)

To start a local Postgres database:
```bash
docker compose -f ../../deploy/docker/docker-compose.yml up -d
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection string | `postgres://postgres:password@localhost:5432/nimbusdb?sslmode=disable` |
| `PORT` | HTTP REST Server port | `8080` |
| `GRPC_PORT` | gRPC Server port | `50051` |

### Running the Service

The service automatically applies all schema migrations and seeds default regions/clusters on startup.

```bash
DATABASE_URL="postgres://postgres:password@localhost:5432/nimbusdb?sslmode=disable" PORT=8080 GRPC_PORT=50051 go run main.go
```

Verify service is running:
```bash
curl http://localhost:8080/health
```

### Running Tests

To run the unit and integration-level schema tests:
```bash
$env:DATABASE_URL="postgres://postgres:password@localhost:5432/nimbusdb?sslmode=disable"
go test -v ./...
```

---

## Known Gaps (Not Yet Implemented)

- **Database Provisioning**: Actual logical databases are not provisioned (Phase 2).
- **Storage Engine**: Actual page data/WAL processing does not exist (Phase 3).
- **Multi-Region**: Cross-region latency and consistency routing are stubbed/deferred (Phase 4).
