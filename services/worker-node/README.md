# NimbusDB Worker Node (Simulated Client)

This service is the simulated node agent running on worker hosts. It interacts with the NimbusDB Control Plane (Metadata Service) over gRPC.

## Responsibilities

- Connects to the Metadata Service via gRPC on startup.
- Registers itself to a specific database cluster using the `RegisterNode` API.
- Runs a 5-second random-walk health heartbeat loop (`SendHeartbeat` gRPC API) reporting:
  - CPU Usage %
  - Memory Usage %
  - Disk Space %
- Hosts a gRPC server (`NodeAgent` service) executing:
  - `CreateDatabase`: Allocates local disk namespace directories (`data/<db_id>`) and returns client endpoint.
  - `DeleteDatabase`: Deletes database namespace directories.
  - `BackupDatabase`: Stubbed returning `codes.Unimplemented` in Phase 2.
  - `RestoreDatabase`: Stubbed returning `codes.Unimplemented` in Phase 2.
- Hosts a debug HTTP server to pause/resume heartbeats and inject simulated provisioning failures for E2E testing.

## Getting Started

### Prerequisites

- Go 1.25+
- Running Metadata Service gRPC server (defaults to port `50051`).

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `METADATA_GRPC_ADDR` | Metadata Service gRPC address | `localhost:50051` |
| `CLUSTER_ID` | UUID of the cluster to register with | `00000000-0000-0000-0000-000000000000` |
| `HOSTNAME` | Name of the worker node | `worker-local` |
| `DEBUG_PORT` | Port for the HTTP debug control endpoints | `8081` |
| `NODE_AGENT_PORT` | Port for the NodeAgent gRPC server | `50053` |

### Running the Worker Node

```bash
METADATA_GRPC_ADDR="localhost:50051" CLUSTER_ID="00000000-0000-0000-0000-000000000000" HOSTNAME="worker-1" DEBUG_PORT=8081 NODE_AGENT_PORT=50053 go run main.go
```

### Debug Endpoints

You can simulate node failure or recovery by pausing or resuming the heartbeat loop, or injecting provisioning failures:

- **Pause Heartbeats**:
  ```bash
  curl -X POST http://localhost:8081/debug/pause
  ```
- **Resume Heartbeats**:
  ```bash
  curl -X POST http://localhost:8081/debug/resume
  ```
- **Inject Provisioning Failure (fails next N CreateDatabase calls)**:
  ```bash
  curl -X POST "http://localhost:8081/debug/inject-failure?attempts=1"
  ```
- **Inject Provisioning Hang (next N CreateDatabase calls sleep 15s)**:
  ```bash
  curl -X POST "http://localhost:8081/debug/inject-failure?hang=1"
  ```

> [!WARNING]
> Simulated failure injection endpoints must never be exposed or triggered outside of testing environments.
