# NimbusDB Worker Node (Simulated Client)

This service is the simulated node agent running on worker hosts. It interacts with the NimbusDB Control Plane (Metadata Service) over gRPC.

## Responsibilities

- Connects to the Metadata Service via gRPC on startup.
- Registers itself to a specific database cluster using the `RegisterNode` API.
- Stores the returned Node ID locally.
- *Future*: Periodic health heartbeat loop (sending synthetic CPU/RAM/Disk stats), handling database provisioning stubs (Create/Delete/Backup/Restore).

## Getting Started

### Prerequisites

- Go 1.22+
- Running Metadata Service gRPC server (defaults to port `50051`).

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `METADATA_GRPC_ADDR` | Metadata Service gRPC address | `localhost:50051` |
| `CLUSTER_ID` | UUID of the cluster to register with | `00000000-0000-0000-0000-000000000000` |
| `HOSTNAME` | Name of the worker node | `worker-local` |

### Running the Worker Node

```bash
METADATA_GRPC_ADDR="localhost:50051" CLUSTER_ID="<VALID_CLUSTER_UUID>" HOSTNAME="worker-1" go run main.go
```

Logs will output:
```json
{"time":"2026-07-13T02:15:00.000Z","level":"INFO","msg":"starting nimbusdb worker node"}
{"time":"2026-07-13T02:15:00.000Z","level":"INFO","msg":"connecting to metadata service","address":"localhost:50051"}
{"time":"2026-07-13T02:15:00.000Z","level":"INFO","msg":"registering node with metadata service","cluster_id":"<VALID_CLUSTER_UUID>","hostname":"worker-1"}
{"time":"2026-07-13T02:15:00.000Z","level":"INFO","msg":"node registered successfully","node_id":"<RETURNED_NODE_UUID>","heartbeat_interval_seconds":5}
{"time":"2026-07-13T02:15:00.000Z","level":"INFO","msg":"worker node is running, waiting for signal..."}
```

---

## Known Gaps (Not Yet Implemented)

- **Heartbeat loop**: Periodic statistics sending (every 5 seconds) is deferred (Step 3).
- **Provisions handlers**: Stub APIs for backup, restore, database layout adjustments are deferred (Phase 2).
- **Graceful deregister**: Deregistering the node from metadata cluster state on shutdown (SIGTERM).
