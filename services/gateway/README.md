# API Gateway (`services/gateway/`)

The **API Gateway** serves as the client-facing front door (REST API) for NimbusDB, replacing direct client calls to the Control Plane. It performs nearest-region routing, transparent region failover rerouting, and aggregates regional health telemetry.

## Responsibilities

- **REST Client Edge**: Exposes `/v1/databases`, `/v1/regions`, and `/health` REST endpoints.
- **Nearest-Region Routing**: Evaluates client `preferredRegion` hints against synthetic inter-region latency matrices (`india`, `us-east`, `us-west`, `europe`, `japan`).
- **Transparent Region Failover**: Reroutes client requests seamlessly to the next nearest healthy region if the requested primary region is `DOWN`.

## API Routes

| Method | Path | Description |
|---|---|---|
| `POST` | `/v1/databases` | Provision database with region hint & transparent fallback |
| `GET` | `/v1/databases/{id}` | Get database status & location |
| `GET` | `/v1/databases` | List all registered databases |
| `DELETE` | `/v1/databases/{id}` | Delete database record |
| `GET` | `/v1/regions` | List region health statuses & latency matrix |
| `GET` | `/health` | Liveness health check |

## Running Tests

```bash
go test -v ./...
```
