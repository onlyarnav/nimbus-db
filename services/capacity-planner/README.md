# NimbusDB Capacity Planner Service

## Overview
`capacity-planner` is an operational service in NimbusDB that analyzes historical cluster load metrics to forecast near-term infrastructure capacity requirements (e.g. projected node count over a 7-day or 30-day horizon).

## Forecasting Method: Linear Regression (Deliberate Simplification)
Per the design decision in `docs/decisions/capacity-planning-method.md`, `capacity-planner` deliberately uses **linear regression** over historical time-series metric samples rather than a heavy machine learning forecasting model.

- **Formula**: $y = mx + b$ computed via ordinary least squares over historical time points.
- **Rationale**: Provides a clear, deterministic, and whiteboard-defensible metric projection without adding complex external ML framework dependencies to the operational control plane.

## Protocol & Interface
- **Internal RPC**: gRPC on port `50059` (`CapacityPlannerService`).
- **Telemetry & Health**: HTTP on port `8089` (`/health`, `/metrics`).
