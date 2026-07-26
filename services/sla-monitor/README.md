# NimbusDB SLA Monitor Service

## Overview
`sla-monitor` tracks service level objectives (SLOs) across the NimbusDB platform over a rolling observation window:
- **Availability**: Calculates overall system availability percentage ($ \frac{\text{Total} - \text{Failed}}{\text{Total}} \times 100 \% $).
- **Latency Percentiles**: Aggregates p95 and p99 request latencies.
- **Recovery Time (MTTR)**: Tracks mean time to recovery from node/region failure events.
- **SLO Verification**: Validates whether cluster metrics meet the target **99.9% availability SLO**.

## Protocol & Interface
- **Internal RPC**: gRPC on port `50060` (`SLAMonitorService`).
- **Telemetry & Health**: HTTP on port `8090` (`/health`, `/metrics`).
