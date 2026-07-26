# NimbusDB Deployment Controller Service

## Overview
`deployment-controller` is an independent operational microservice in NimbusDB responsible for orchestrating application-level deployment strategies and safe node lifecycle operations:
- **Rolling Deployments**: Updates nodes in sequence, performing health check validations and halting immediately if an instance fails.
- **Canary Deployments**: Provisions canary versions to a traffic subset, monitors error metrics, and triggers **automatic rollbacks** if metric thresholds are breached.
- **Blue-Green Deployments**: Provisions parallel environments and performs atomic traffic switches upon verification.
- **Node Draining**: Coordinates marking nodes `draining`, evacuating active workloads without data loss, and finalizing node shutdown.

## Protocol & Interface
- **Internal RPC**: gRPC on port `50058` (`DeploymentControllerService`).
- **Telemetry & Health**: HTTP on port `8088` (`/health`, `/metrics`).

## Known Limitations / Scope Boundaries
- **App-level Orchestration**: Operates at the application control plane level; does not wrap Kubernetes primitives (K8s operator mechanics are reserved for Phase 9).
- **No Cost Modeling**: Deployment and scaling decisions are based purely on health and performance metrics, not cloud provider cost models.
