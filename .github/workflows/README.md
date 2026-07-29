# NimbusDB CI/CD Pipeline Documentation

## Overview
This directory contains the GitHub Actions workflows for continuous integration, continuous deployment, and automated rollback for the NimbusDB Distributed AI-Ready Cloud Database Platform.

## Pipeline Architecture

```
Push / PR  ──►  ci.yml (Lint, Unit Tests, Integration Tests, Docker Build, Helm Validate)
                    │
                    ▼ (Merge to main)
                cd.yml (Image Push, Helm Upgrade, Phase 7 Rollout, Post-Deploy Smoke Test)
                    │
           ┌────────┴────────┐
           ▼                 ▼
     (All Healthy)    (Health Check Fails)
  Phase 5 Webhook        rollback.yml (Helm Rollback, Health Re-Check, Alert Payload)
```

## Workflows Breakdown

### 1. `ci.yml` (Continuous Integration)
- **Triggers**: Pushes on any branch, Pull Requests to `main`.
- **Stages**:
  - `lint`: Runs `go vet` and static code analysis.
  - `unit-tests`: Runs unit tests across all 10 microservices.
  - `integration-tests`: Runs full integration test suite (`tests/integration/`).
  - `docker-build`: Validates multi-stage Docker builds.
  - `helm-validation`: Runs `helm lint` and `helm template` on `deploy/helm/nimbusdb`.

### 2. `cd.yml` (Continuous Deployment)
- **Triggers**: Merge to `main`.
- **Stages**:
  - Pushes commit-SHA-tagged Docker images to container registry.
  - Deploys via `helm upgrade` and invokes Phase 7 `deployment-controller` canary/rolling rollout.
  - Executes post-deploy `/health` polling and live E2E smoke tests.
  - On success: Sends Phase 5 Webhook Receiver notification.
  - On failure: Automatically dispatches `rollback.yml`.

### 3. `rollback.yml` (Automated & Manual Rollback)
- **Triggers**: Automatic invocation by `cd.yml` on failure, or manual `workflow_dispatch`.
- **Stages**:
  - Executes `helm rollback` to the previous known-good release revision.
  - Re-checks `/health` endpoints and re-runs live smoke tests to verify health restoration.
  - Emits structured JSON log entry and fires Phase 5 Alertmanager webhook.

## Branch Protection Configuration
To enforce pipeline safety on GitHub:
1. Navigate to **Settings** -> **Branches** -> **Add branch protection rule**.
2. Set **Branch name pattern** to `main`.
3. Check **Require a pull request before merging**.
4. Check **Require status checks to pass before merging** and select:
   - `Code Quality & Linting`
   - `Unit Test Suite`
   - `Integration Test Suite`
   - `Docker Image Build Validation`
   - `Helm Chart Validation`
5. Check **Require branches to be up to date before merging**.

## Manual Rollback Instructions
To manually trigger a rollback to the previous release revision:
```bash
gh workflow run rollback.yml -f reason="Operator manual rollback due to external incident"
```
Or via GitHub UI: **Actions** tab -> **Automated & Manual Rollback** -> **Run workflow**.
