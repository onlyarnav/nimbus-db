# NimbusDB Worker Node (Simulated Client)

This service is the simulated node agent running on worker hosts. It interacts with the NimbusDB Control Plane (Metadata Service) over gRPC.

## Responsibilities

- Connects to the Metadata Service via gRPC on startup.
- Registers itself to a specific database cluster using the `RegisterNode` API.
- Runs a 5-second random-walk health heartbeat loop (`SendHeartbeat` gRPC API) reporting:
  - CPU Usage %
  - Memory Usage %
  - Disk Space %
- Hosts a debug HTTP server (listening on port `8081` by default) to pause and resume heartbeats for E2E failure testing.

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

### Running the Worker Node

```bash
METADATA_GRPC_ADDR="localhost:50051" CLUSTER_ID="00000000-0000-0000-0000-000000000000" HOSTNAME="worker-1" DEBUG_PORT=8081 go run main.go
```

### Debug Endpoints

You can simulate node failure or recovery by pausing or resuming the heartbeat loop:

- **Pause Heartbeats**:
  ```bash
  curl -X POST http://localhost:8081/debug/pause
  ```
- **Resume Heartbeats**:
  ```bash
  curl -X POST http://localhost:8081/debug/resume
  ```
