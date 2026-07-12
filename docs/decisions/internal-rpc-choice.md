# Internal RPC Choice: gRPC

## Context and Problem Statement

NimbusDB services (Metadata Service, Scheduler, Node Agent/Storage Engine) need to communicate over the network. We need to define the protocol for internal service-to-service calls.

## Decision

We chose **gRPC** for all internal service-to-service communication.

## Rationale

1. **Typed Contracts**: Enforces strict protobuf schemas across Go and Rust/C++ services, preventing runtime serialization errors.
2. **Advanced Capabilities**: Needed for streaming later, such as real-time heartbeat streams, replication logs, and distributed tracing context propagation.
3. **Architecture Match**: Matches Azure-native microservice communication patterns.
4. **Dashboard Boundary**: REST is reserved only for the dashboard-facing edge (external client to control plane router).
