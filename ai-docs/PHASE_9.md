# PHASE_9.md — Kubernetes Deployment

Companion spec to `GEMINI.md`, `PHASE_1.md`–`PHASE_8.md`. Do not start
this phase until Phase 8's acceptance criteria are fully met (see
`PROJECT_STATUS.md`). This phase assumes a fully secured, observable,
multi-region cluster with cloud-ops tooling already in place — all
running via Docker/docker-compose up to this point. This phase does not
change any service's logic; it changes how they're packaged and deployed.

**Scope boundary — read this first:** Phase 7 already built an
application-level deployment controller (rolling/canary/blue-green,
Section 3 of `PHASE_7.md`). This phase does **not** duplicate that logic
in Kubernetes-native form as a competing system — it wraps the existing
services in K8s primitives (Deployments, Services, HPA) for standard
container orchestration, while Phase 7's deployment controller remains the
layer that makes *application-aware* rollout decisions (canary metric
evaluation, etc.). Where the two overlap (e.g., K8s rolling update vs
Phase 7's rolling deployment), document explicitly which one is
authoritative and why, rather than silently running both.

---

## 1. Goal of This Phase

Package and deploy every service from Phases 1-8 onto Kubernetes: proper
Helm charts, Ingress for external traffic, and Horizontal Pod Autoscaling
wired to the same signals Phase 7's auto-scaler uses.

**Definition of done:** `helm install` brings up the entire working
cluster from a clean state — every service, correctly configured,
correctly networked, passing health checks — verified end-to-end, not
just "the pods started."

---

## 2. Services Touched in This Phase

```
deploy/
├── docker/            ← Dockerfiles for every service (should mostly
│                          already exist from earlier phases' docker-
│                          compose setups — audit and consolidate, don't
│                          rebuild from scratch)
└── helm/
    ├── nimbusdb/         ← umbrella chart
    │   ├── Chart.yaml
    │   ├── values.yaml
    │   └── templates/
    └── charts/              ← per-service subcharts (or templates within
                                the umbrella chart — pick one pattern,
                                document it)
```

No new application code in this phase — pure packaging/infra work. If
building the Helm charts surfaces a service that doesn't have a working
Dockerfile yet, treat that as a gap from an earlier phase to fix, not new
Phase 9 scope creep.

---

## 3. Dockerfiles (audit + consolidate)

### 3.1 Requirements Per Service
- Multi-stage build (build stage + minimal runtime stage) — do not ship
  build tooling/source in the final image.
- Non-root user in the runtime stage.
- Explicit `/health` endpoint exposed for K8s liveness/readiness probes
  (already exists per `GEMINI.md` Section 2, item 7 — this phase just
  wires it into K8s probe config, doesn't invent it).

### 3.2 Acceptance
- Every service builds a working image (`docker build` succeeds).
- Image size is reasonable for its language (flag anything unexpectedly
  large — usually a sign the multi-stage build isn't discarding build
  artifacts correctly).

---

## 4. Helm Charts

### 4.1 Per-Service Chart Contents
For each service (metadata-service, worker-node/node-agent, scheduler,
control-plane, gateway, dashboard, observability stack components,
deployment-controller, capacity-planner, sla-monitor, auth-service):
- `Deployment` (or `StatefulSet` where appropriate — see Section 4.2)
- `Service` (ClusterIP for internal-only services, matches the
  gRPC-internal/REST-edge split already established)
- `ConfigMap` for non-secret config
- Reference to `Secret` for secrets (created via the Section 7 secrets
  approach from `PHASE_8.md` — do not invent a new secrets mechanism here,
  wire K8s Secrets to the same values)
- Resource requests/limits (CPU/memory) — set real, sensible values based
  on what was observed in Phase 5's metrics during testing, not arbitrary
  placeholders

### 4.2 StatefulSet vs Deployment
- Metadata Service's Postgres backend (if self-hosted rather than a
  managed external Postgres) needs a `StatefulSet` with persistent volume
  claims — stateful, needs stable identity/storage.
- Node Agent (storage engine) similarly needs a `StatefulSet` + PVC per
  replica — this is where Phase 3's actual data lives, it cannot be a
  stateless `Deployment`.
- Stateless services (Scheduler, Control Plane, Gateway, Dashboard) use
  regular `Deployment`.
- Document this distinction explicitly in `docs/decisions/k8s-workload-types.md`
  — getting this wrong (e.g., treating the storage engine as stateless) is
  a common and consequential K8s mistake worth being deliberate about.

### 4.3 Values File Structure
`values.yaml` should parameterize: replica counts, resource limits, image
tags, region configuration (mapping to the simulated regions from Phase
4 — consider whether regions map to K8s namespaces, clusters, or just
labels/node-affinity rules within one cluster; pick one and document it,
since a single local/dev K8s cluster can't literally run 5 geographic
regions).

---

## 5. Ingress

### 5.1 Design
- Route external traffic to the Gateway (Phase 4) only — no other service
  should be externally reachable, consistent with the REST-edge/gRPC-
  internal boundary maintained since Phase 1.
- TLS termination at Ingress (ties into Phase 8's Section 8.1 in-transit
  encryption decision — use the same certs/approach, don't introduce a
  second TLS setup).

### 5.2 Acceptance
- External requests only succeed through the Ingress-routed Gateway path;
  direct attempts to reach internal services from outside the cluster
  network fail (verify this, don't just assume `ClusterIP` scoping is
  sufficient without checking).

---

## 6. Horizontal Pod Autoscaler (HPA)

### 6.1 Design
- Apply HPA to the stateless, horizontally-scalable services (Node Agent
  worker pool, Scheduler, Gateway) based on CPU/memory metrics — the same
  signals Phase 7's application-level auto-scaler (Section 5 of
  `PHASE_7.md`) already uses.
- **Explicit decision needed:** does K8s HPA replace Phase 7's
  auto-scaling logic, or do they operate at different layers (HPA scales
  pod count, Phase 7's logic makes data-placement-aware decisions about
  which nodes host which databases)? Recommendation: keep both — HPA
  handles raw pod-count elasticity, Phase 7's scheduler-integrated logic
  handles placement within the resulting pod set. Document this
  explicitly in `docs/decisions/hpa-vs-app-autoscaler.md` so it doesn't
  read as redundant/confused scope.

### 6.2 Acceptance
- Triggering load (reuse Phase 7's load-generation test tooling) causes
  HPA to scale pod replicas, verified via `kubectl get hpa` / pod count
  before and after.

---

## 7. Testing Requirements

### 7.1 Validation (not traditional unit tests — this phase is
infrastructure, tested differently)
- `helm lint` passes on every chart.
- `helm template` renders without errors across the full values matrix
  (at minimum: default values, and a "production-like" values override).

### 7.2 Integration Test (required — this is the phase's acceptance proof)
1. **Clean-state install test (the core deliverable, matches GEMINI.md's
   exact requirement):** on a clean K8s cluster (kind/minikube acceptable
   for local verification), `helm install` the umbrella chart, wait for
   all pods to reach `Ready`, confirm every service's `/health` reports
   healthy.
2. **End-to-end smoke test:** run a subset of earlier phases' integration
   tests (e.g., Phase 1's cluster-registration flow, Phase 2's database
   creation) against the K8s-deployed system, not just against
   docker-compose — confirms the K8s packaging didn't break actual
   functionality, not just that pods started.
3. **Ingress isolation test:** confirm internal services are unreachable
   from outside the cluster except via the Gateway/Ingress path (Section
   5.2).
4. **HPA scale test** (Section 6.2).
5. **Statefulness test:** kill a Node Agent pod backed by a `StatefulSet` +
   PVC, confirm it comes back with its data intact (this is Phase 3's
   crash-consistency guarantee, now proven under K8s's pod-restart model
   specifically, not just a bare process restart).

### 7.3 What "done" looks like
- [ ] `helm lint` and `helm template` pass on all charts
- [ ] Clean-state install test passes — full cluster comes up healthy from
      `helm install` alone
- [ ] End-to-end smoke test (reusing earlier phases' test logic) passes
      against the K8s deployment
- [ ] Ingress isolation, HPA scale, and statefulness tests all pass
- [ ] `docs/decisions/k8s-workload-types.md` and
      `docs/decisions/hpa-vs-app-autoscaler.md` written
- [ ] `docs/benchmarks.md` updated with: time from `helm install` to fully
      healthy cluster, HPA scale-out latency under load — measured
- [ ] `deploy/helm/README.md` documents how to install, upgrade, and
      uninstall the chart, plus the values.yaml parameters
- [ ] `PROJECT_STATUS.md` updated to mark Phase 9 complete — only once
      every item above is actually done

---

## 8. Explicit Non-Goals for Phase 9

- Multi-cluster/multi-region K8s federation (regions are simulated within
  a single cluster via namespace/affinity, per Section 4.3 — true
  geo-distributed K8s federation is out of scope)
- Service mesh (Istio/Linkerd) — not required to demonstrate the
  concepts this project targets; mTLS via K8s-native means (Section 5.1)
  is sufficient
- GitOps tooling (ArgoCD/Flux) — that's arguably CI/CD-adjacent and can be
  mentioned as a natural Phase 10 extension, not required here
- Production-grade cluster provisioning (Terraform/cloud IaC for the K8s
  cluster itself) — this phase assumes a K8s cluster already exists
  (local kind/minikube for verification is sufficient)

---

## 9. Suggested Build Order (within this phase)

1. Dockerfile audit across all services (Section 3) — fix any missing/
   broken ones before Helm work starts.
2. StatefulSet vs Deployment decision logged (Section 4.2 — gate before
   writing charts for stateful services).
3. Helm charts for stateless services first (Scheduler, Control Plane,
   Gateway, Dashboard) — simpler, de-risks the charting process.
4. Helm charts for stateful services (Metadata Service/Postgres, Node
   Agent) with PVCs.
5. Umbrella chart wiring everything together, `values.yaml` structure.
6. Ingress (Section 5).
7. Clean-state install test (Section 7.2, item 1) — the phase's real
   milestone. Do not proceed until this is solid.
8. End-to-end smoke test against K8s (Section 7.2, item 2).
9. HPA (Section 6) + scale test.
10. Statefulness test (Section 7.2, item 5).
11. Benchmarks + decision docs + README + `PROJECT_STATUS.md` update.

Work through these one at a time. After each numbered step, report status
before moving to the next, per the working agreement in `GEMINI.md`.
