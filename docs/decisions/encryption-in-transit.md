# Architectural Decision Record: Encryption in Transit & TLS Strategy

## Context
In Phase 8, we must establish encryption in transit rules across external REST endpoints and internal gRPC service-to-service calls.

## Decisions

### 1. External REST Edge TLS
- **Edge Security**: HTTPS/TLS enabled on the Gateway REST edge (`https://localhost:8443` or `https://localhost:8080`).
- **Certificates**: Self-signed TLS 1.3 certificates used in local development/testing sandbox environment. Production deployment uses Let's Encrypt / ACME cert-manager certificates on Ingress (Phase 9).

### 2. Internal gRPC Service-to-Service Traffic
- **Trusted Network Assumption**: Internal gRPC traffic between `Gateway`, `Control Plane`, `Metadata Service`, `Scheduler`, `Worker Nodes`, `Deployment Controller`, `Capacity Planner`, and `SLA Monitor` operates within a trusted, isolated private cluster network (Docker bridge network / Kubernetes pod CIDR).
- *Rationale*: Terminating TLS at the Gateway REST edge and relying on network isolation for high-performance internal gRPC streaming is a standard production design pattern that avoids double-encryption CPU overhead while keeping internal calls secure against external ingress.
