# NimbusDB Scheduler Service

The Scheduler is a standalone control-plane microservice that determines node placement for database deployments. It uses a weighted resource capacity algorithm to schedule databases on the least-loaded nodes.

## Responsibilities

- Exposes the `Schedule` gRPC API (listening on port `50052`).
- Communicates with the Metadata Service to query the current node topologies and resources.
- Evaluates candidate nodes using the **Least Loaded Algorithm**.

## Tech Stack

- **Language**: Go 1.25.0
- **Internal API**: gRPC (port `50052`)
- **Metadata client**: gRPC client to query Metadata Service (`50051`)

## Least Loaded Scoring Formula

The scheduler scores candidate nodes using a weighted average of free CPU, Memory, and Disk resources:

$$\text{Score} = (100 - \text{CPU Pct}) \times 0.4 + (100 - \text{Memory Pct}) \times 0.3 + (100 - \text{Disk Pct}) \times 0.3$$

### Decision Tree Hierarchy

1. **Exclusions**: Node candidate list filters out any nodes with a status of `dead` or `draining`.
2. **Prioritization**:
   - The scheduler prefers nodes that are **not** `overloaded` (defined as resource usage >90%).
   - If non-overloaded nodes exist, the node with the highest score is returned.
   - If all candidate nodes are `overloaded`, the scheduler schedules on the highest-scoring overloaded node as a fallback.
   - If no nodes remain healthy/candidate, it returns a `ResourceExhausted` gRPC error code.

## Getting Started

### Prerequisites

- Go 1.25+
- Running Metadata Service gRPC server (port `50051`).

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `METADATA_GRPC_ADDR` | Metadata Service gRPC address | `localhost:50051` |
| `SCHEDULER_PORT` | Scheduler gRPC Server port | `50052` |

### Running the Service

```bash
METADATA_GRPC_ADDR="localhost:50051" SCHEDULER_PORT=50052 go run main.go
```
