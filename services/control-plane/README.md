# NimbusDB Control Plane Service

The Control Plane manages metadata placement, API orchestration, and database scheduling decisions. It hosts the external REST API endpoints for database lifecycle operations, translating them into internal gRPC calls.

## Responsibilities

- **Internal gRPC API Service**: Serves internal provisioning and region failover orchestration requests from the Gateway (`services/gateway`).
- **Placement Scheduling**: Queries the Scheduler service (via gRPC) to find the least loaded worker node within target/fallback regions.
- **Provisioning Coordination**: Directs Node Agents (via gRPC) to allocate directory namespaces on selected workers.
- **Multi-Region Leader Election & Failover**: Orchestrates deterministic leader election when a primary region goes down, promoting the highest-LSN follower region to primary.
- **Resilient Retry Orchestrator**: Handles mid-provision failures with automatic node failover and rescheduling (reschedules up to 3 times).
- **Background Reconciler Loop**: Periodically sweeps the Registry to identify database health state changes and execute automatic region failovers.


> [!NOTE]
> **Explicit Stubbing**: In Phase 2, `BackupDatabase` and `RestoreDatabase` gRPC endpoints are intentionally left as unimplemented stubs returning `codes.Unimplemented`. Full integration will take place in Phase 3 once the underlying WAL and snapshot storage mechanisms are developed.

## Getting Started

### Prerequisites

- Go 1.25+
- Running Metadata Service (defaults to `localhost:50051`)
- Running Scheduler Service (defaults to `localhost:50052`)

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `HTTP_PORT` | HTTP port for the client REST API | `8082` |
| `METADATA_GRPC_ADDR` | Metadata Service gRPC endpoint address | `localhost:50051` |
| `SCHEDULER_GRPC_ADDR` | Scheduler Service gRPC endpoint address | `localhost:50052` |

### Running the Control Plane

```bash
HTTP_PORT=8082 METADATA_GRPC_ADDR="localhost:50051" SCHEDULER_GRPC_ADDR="localhost:50052" go run .
```

## REST API Specification

### 1. Create Database
- **Method & Path**: `POST /v1/databases`
- **Request Body**:
  ```json
  {
    "name": "orders-db",
    "clusterId": "00000000-0000-0000-0000-000000000000"
  }
  ```
- **Response (202 Accepted)**:
  ```json
  {
    "databaseId": "1b08f4c2-9e90-48a3-a6d1-cf19985b8ff3",
    "status": "provisioning"
  }
  ```

### 2. Get Database Status/Location
- **Method & Path**: `GET /v1/databases/{id}`
- **Response (200 OK)**:
  ```json
  {
    "databaseId": "1b08f4c2-9e90-48a3-a6d1-cf19985b8ff3",
    "name": "orders-db",
    "status": "active",
    "nodeId": "8f8bca12-a1f9-4b82-aa8c-d68f2bc0e50f",
    "endpoint": "worker-1/db/1b08f4c2-9e90-48a3-a6d1-cf19985b8ff3",
    "attempts": 1,
    "createdAt": "2026-07-20T18:00:00Z"
  }
  ```

### 3. List Databases
- **Method & Path**: `GET /v1/databases` (optional: `?clusterId=...`)
- **Response (200 OK)**:
  ```json
  [
    {
      "databaseId": "1b08f4c2-9e90-48a3-a6d1-cf19985b8ff3",
      "name": "orders-db",
      "status": "active",
      "nodeId": "8f8bca12-a1f9-4b82-aa8c-d68f2bc0e50f",
      "endpoint": "worker-1/db/1b08f4c2-9e90-48a3-a6d1-cf19985b8ff3",
      "attempts": 1,
      "createdAt": "2026-07-20T18:00:00Z"
    }
  ]
  ```

### 4. Delete Database
- **Method & Path**: `DELETE /v1/databases/{id}`
- **Response (200 OK)**:
  ```json
  {
    "success": true
  }
  ```

### 5. Health Liveness Probe
- **Method & Path**: `GET /health`
- **Response (200 OK)**:
  ```json
  {
    "status": "UP"
  }
  ```
