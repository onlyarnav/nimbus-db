# Service Communication Choice: gRPC

## Context and Problem Statement

NimbusDB services (Metadata Service, Scheduler, Node Agent/Storage Engine) need to communicate over the network. We need to choose the protocol for internal service-to-service calls.

We evaluated two options:
1. **REST / HTTP JSON**: Simple, easy to test, and widely compatible, but lacks strict schema enforcement out of the box and requires manually written JSON serialization/deserialization.
2. **gRPC / Protocol Buffers**: Strongly typed API contracts, binary serialization for efficiency, and code generation support for multiple languages (Go, Rust/C++).

## Decision

We chose **gRPC** for all internal control-plane service-to-service communication.

## Rationale

1. **Strict Type Safety**: Defining the API contract in Protobuf ensures compile-time safety and automatic code generation across different languages (e.g. Go for Metadata/Scheduler and Rust/C++ for Node Agent).
2. **Performance**: Binary serialization via Protocol Buffers reduces overhead compared to text-based JSON.
3. **API Integrity**: Promotes clear versioning and prevents schema drift between services in the distributed system.
4. **Dashboard Boundary**: The Dashboard (Next.js) will communicate with the Metadata Service via a REST gateway or REST endpoints, but the main internal mesh will use gRPC.
