# NimbusDB Helm Chart (`deploy/helm/nimbusdb`)

## Overview
This directory contains the umbrella Helm chart for deploying the NimbusDB Distributed AI-Ready Cloud Database Platform to Kubernetes.

## Chart Architecture & Workload Types
- **StatefulSets (`StatefulSet` + PVC)**:
  - `metadata-postgres`: Relational database metadata store.
  - `worker-node`: Storage engine (Phase 3 4KB pages, B+Tree indexes, WAL, vector graph indexes).
- **Deployments (`Deployment`)**:
  - `gateway`, `auth-service`, `scheduler`, `control-plane`, `deployment-controller`, `capacity-planner`, `sla-monitor`, `dashboard`.
- **Ingress**: External TLS edge routing traffic to `gateway` only (`nimbusdb.local`).
- **Autoscaling**: K8s HPA configured for `gateway`, `scheduler`, and `worker-node`.

## Installation & Deployment Commands

### 1. Validate Chart
```bash
helm lint deploy/helm/nimbusdb
helm template release-test deploy/helm/nimbusdb
```

### 2. Install Chart
```bash
helm install nimbusdb deploy/helm/nimbusdb --namespace nimbusdb --create-namespace
```

### 3. Upgrade & Uninstall
```bash
helm upgrade nimbusdb deploy/helm/nimbusdb --namespace nimbusdb
helm uninstall nimbusdb --namespace nimbusdb
```
