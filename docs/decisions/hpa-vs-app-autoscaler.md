# Architectural Decision Record: HPA vs Application-Level Auto-Scaler Layering

## Context
Phase 7 implemented an application-level `AutoScaler` inside the `scheduler` service, while Phase 9 introduces Kubernetes-native `HorizontalPodAutoscaler` (HPA). We must clarify the operational boundaries to prevent overlapping or conflicting scaling behavior.

## Decision & Layering Strategy
NimbusDB retains **both autoscaling mechanisms operating at distinct system layers**:

1. **Infrastructure Elasticity Layer (`Kubernetes HPA`)**:
   - *Scope*: Operates at the Kubernetes control plane layer.
   - *Function*: Automatically scales pod replica counts for stateless services (`Gateway`, `Scheduler`, `Worker Node` pool) based on raw CPU and Memory resource consumption thresholds.
   - *Target*: Pod replica count management.

2. **Application Placement & Workload Layer (`Phase 7 AutoScaler`)**:
   - *Scope*: Operates at the NimbusDB control plane / database management layer.
   - *Function*: Evaluates data-placement aware metrics (database allocation density, replica health, node draining status). When node scale-in occurs, Phase 7 logic executes zero-loss database evacuation and node status transitions (`draining` -> `drained`) before pod deprovisioning.
   - *Target*: Database partition relocation, node draining, and replica placement.
