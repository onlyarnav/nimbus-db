# Architectural Decision Record: Auto-Scaling Thresholds & Cooldown Strategy

## Context
Phase 7 introduces application-level auto-scaling capabilities in the NimbusDB `scheduler` microservice (`services/scheduler/placement/autoscaler.go`). The auto-scaler continuously evaluates worker node CPU and memory utilization statistics received from Metadata Service heartbeats to determine when the cluster should scale out (provision additional worker nodes) or scale in (drain and deprovision underutilized worker nodes).

We must select concrete default threshold percentages for scaling decisions and define a cooldown period to prevent resource flapping.

## Decisions

### 1. Scaling Threshold Defaults
- **Scale-Out Trigger**: Cluster average CPU or Memory utilization **$\ge 75.0\%$**.
  - *Rationale*: Triggering scale-out at 75% ensures newly provisioned nodes register and become available before worker nodes become overloaded ($\ge 90\%$).
- **Scale-In Trigger**: Cluster average CPU and Memory utilization **$\le 20.0\%$** across active worker nodes (only evaluated when active node count $> 1$).
  - *Rationale*: A conservative 20% threshold prevents premature scale-in during minor load dips while allowing consolidation of idle worker nodes.

### 2. Cooldown Period Strategy
- **Cooldown Window**: Default **10 seconds** for test/demo environments (configurable up to 300 seconds for production).
- *Rationale*: After executing a scale-out or scale-in action, the auto-scaler ignores load fluctuations during the cooldown window. This prevents flapping (repeated rapid scale-out and scale-in cycles caused by transient metric bursts).
