# Architectural Decision Record: Release Strategy (`Merge-to-Main Continuous Deployment`)

## Context
When establishing automated CI/CD for NimbusDB, we must select a release strategy (merge-to-main continuous deployment vs tag-based releases).

## Decision & Rationale
NimbusDB adopts **Merge-to-Main Continuous Deployment**:

1. **Automation & Speed**: Every commit merged into `main` automatically triggers the full CI/CD pipeline (`ci.yml` -> `cd.yml`), executing Docker image builds, Helm chart deployment, application rollout via Phase 7's deployment controller, and post-deploy live smoke testing without manual intervention.
2. **Safety Net Guaranteed**: Deployments are protected by automated post-deploy health check polling (`/health`) and live smoke tests. In the event of a runtime failure, `rollback.yml` automatically executes `helm rollback` to the previous known-good release revision and re-verifies health.
3. **Always-Green Main**: Branch protection rules require CI status check passes (`lint`, `unit-tests`, `integration-tests`, `helm-validation`) before pull requests can be merged into `main`.
