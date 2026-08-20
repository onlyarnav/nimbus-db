# PHASE_10.md — CI/CD

Companion spec to `GEMINI.md`, `PHASE_1.md`–`PHASE_9.md`. Do not start
this phase until Phase 9's acceptance criteria are fully met (see
`PROJECT_STATUS.md`). This is the final phase of the project — it wires
everything built in Phases 1-9 into an automated pipeline: test → build →
deploy → health check → automatic rollback.

**Scope boundary — read this first:** Phase 7 already built an
application-level deployment controller with canary/rollback logic
(`PHASE_7.md` Section 3). Phase 9 wrapped services in Kubernetes. This
phase does not reimplement either — it builds the GitHub Actions pipeline
that *invokes* them at the right point (e.g., a merged PR triggers a
build, then a deploy step that calls into the existing K8s/Helm and
Phase 7 rollback mechanisms, rather than the pipeline containing its own
separate rollback logic). If this phase starts duplicating deployment
logic that already exists, stop and flag it.

---

## 1. Goal of This Phase

Full CI/CD automation: every push/PR runs the complete test suite from
Phases 1-9, builds Docker images on success, deploys to the K8s cluster
(Phase 9), verifies health, and automatically rolls back on failure — with
zero manual steps required for a normal, correct change to go live.

**Definition of done:** a deliberately broken commit is caught by the
pipeline (test failure, build failure, or post-deploy health-check
failure) and never reaches a "deployed" state — demonstrated, not just
configured.

---

## 2. Services/Artifacts Touched in This Phase

```
.github/
└── workflows/
    ├── ci.yml           ← test + build, runs on every push/PR
    ├── cd.yml              ← deploy, runs on merge to main (or tag)
    └── rollback.yml           ← invoked by cd.yml on health-check failure,
                                   or manually triggerable
```

No new application services in this phase — pure pipeline configuration,
plus whatever small scripts/tooling the pipeline needs to glue existing
pieces together (e.g., a health-check-polling script, a script that
triggers Phase 7's rollback endpoint).

---

## 3. CI Pipeline (`ci.yml`)

### 3.1 Triggers
- Every push to any branch, every PR — this is the fast-feedback loop, it
  must run on all proposed changes, not just `main`.

### 3.2 Stages
```
Checkout
  ↓
Lint (per-language: golangci-lint for Go, per GEMINI.md conventions;
      clippy/clang-tidy for the storage engine language chosen in Phase 3)
  ↓
Unit tests (all services, per each phase's unit test suites — this
           should be the accumulated total of every PHASE_N.md's unit
           test requirements, not a new test suite invented for CI)
  ↓
Integration tests (the docker-compose-based integration tests from each
                    phase — these are slower; consider running them as a
                    separate parallel job from unit tests, not serially,
                    to keep CI fast)
  ↓
Build Docker images (per Phase 9's Dockerfiles) — tag with commit SHA,
                       do not push yet on PR builds, only on merge (CD's
                       job)
  ↓
helm lint / helm template validation (Phase 9's charts)
```

### 3.3 Acceptance
- A PR with a failing unit test is blocked from merging (branch protection
  rule, document how it's configured).
- A PR with a linting violation is flagged clearly, not silently ignored.
- CI run time is reasonable — if the full integration test suite across 9
  phases makes CI too slow for practical PR feedback, document a tiering
  strategy (e.g., fast unit tests block merge, full integration suite runs
  on merge to `main` or nightly) rather than silently skipping tests to
  make CI faster. This is a real tradeoff worth being explicit about.

---

## 4. CD Pipeline (`cd.yml`)

### 4.1 Trigger
- Merge to `main` (or a release tag, if you prefer tag-based releases —
  pick one, document in `docs/decisions/release-strategy.md`).

### 4.2 Stages
```
Run CI stages (reuse ci.yml as a prerequisite job, don't duplicate)
  ↓
Push Docker images to registry (tagged with commit SHA + `latest`)
  ↓
Deploy via helm upgrade (Phase 9's charts) — this should invoke Phase 7's
  deployment controller for the actual rollout strategy (rolling/canary),
  not bypass it with a raw `kubectl apply`
  ↓
Post-deploy health check (poll every service's /health, plus a smoke
  test — e.g., Phase 1's cluster-registration flow or Phase 2's database
  creation flow, run against the freshly deployed environment)
  ↓
  ├─ all healthy → deployment complete, notify (Slack/webhook, reuse
  │                  Phase 5's alerting delivery mechanism, don't build
  │                  a second notification path)
  └─ health check fails → trigger rollback.yml automatically
```

### 4.3 Acceptance
- A successful deploy is verified by an actual smoke test against the
  live environment, not just "pods reported Ready" (Phase 9 already
  proved pods-Ready isn't sufficient on its own — reuse that lesson here).

---

## 5. Rollback (`rollback.yml`)

### 5.1 Design
- Triggered automatically by `cd.yml` on health-check failure, or
  manually via `workflow_dispatch`.
- Invokes `helm rollback` to the previous known-good release, then re-runs
  the post-deploy health check to confirm the rollback itself succeeded
  (a rollback that doesn't restore health is a worse failure mode than the
  original one — must be checked, not assumed).
- Records the rollback event as a structured log entry (Phase 5's logging
  convention) and an alert (Phase 5's alerting pipeline).

### 5.2 Acceptance (the phase's core deliverable, matches GEMINI.md's
exact requirement)
- A deliberately broken commit (e.g., a service that fails its own
  `/health` check on startup) pushed through the full pipeline: CI passes
  (if the break is only visible at runtime, not caught by unit tests —
  this is deliberately testing the deploy-time safety net, not the
  test-time one), CD deploys it, the post-deploy health check catches the
  failure, rollback triggers automatically, and the system ends up back
  on the previous good version — verified end-to-end, with timing
  recorded.

---

## 6. Testing Requirements

### 6.1 Validation
- `ci.yml`/`cd.yml`/`rollback.yml` themselves are tested by actually
  running them (GitHub Actions doesn't have a strong local unit-testing
  story for workflows — validate via `act` locally if desired, but the
  real test is Section 6.2).

### 6.2 Integration Test (required — this is the phase's acceptance proof,
and the project's final acceptance proof)
1. **Happy path:** push a correct, working change → confirm it flows
   through CI → CD → deploy → health check → success notification, fully
   automated.
2. **CI-catch test:** push a change with a failing unit test → confirm it
   never reaches the CD pipeline, merge is blocked.
3. **Rollback test (Section 5.2)** — the project's final, highest-value
   demoable artifact. Record this on video/screen-capture if you want a
   portfolio artifact beyond the repo itself — a working automated
   rollback is exactly the kind of thing that's hard to fake and highly
   credible in an interview.

### 6.3 What "done" looks like
- [ ] Happy-path pipeline run succeeds end-to-end
- [ ] CI-catch test confirms broken commits are blocked pre-merge
- [ ] Rollback test (Section 5.2) passes, with the detection-to-rollback
      time measured and recorded
- [ ] `docs/decisions/release-strategy.md` (merge-to-main vs tag-based)
      logged
- [ ] `docs/benchmarks.md` updated with: full pipeline run time (happy
      path), rollback detection-to-recovery time — measured
- [ ] `.github/workflows/README.md` (or equivalent) documents the pipeline
      stages, branch protection setup, and how to manually trigger a
      rollback
- [ ] `PROJECT_STATUS.md` updated to mark Phase 10 complete, **and** the
      overall project status reflects that all 10 phases are done — this
      is the final update to that file for the initial build; from here
      forward it becomes a maintenance/extension log

---

## 7. Explicit Non-Goals for Phase 10

- GitOps tooling (ArgoCD/Flux) — plain GitHub Actions + Helm is sufficient
  to demonstrate the pipeline concepts; GitOps is a legitimate "how this
  would evolve" talking point, not required scope
- Multi-environment promotion pipelines (dev → staging → prod) — this
  project targets a single deployable environment; document
  multi-environment promotion as a natural extension, not build it
- Automated dependency-vulnerability scanning / SAST tooling — worth
  mentioning as a production-readiness gap if asked, but not required for
  this project's core deliverable

---

## 8. Suggested Build Order (within this phase)

1. `ci.yml`: lint + unit tests first (fastest feedback, build this before
   anything else).
2. Add integration tests to CI (Section 3.3's tiering decision — fast vs
   full suite — resolve here).
3. Add Docker build + Helm validation stages to CI.
4. Branch protection configured to require CI success before merge.
5. `cd.yml`: image push + `helm upgrade` deploy stage, invoking Phase 7's
   deployment controller (not bypassing it).
6. Post-deploy health check + smoke test stage.
7. Happy-path pipeline test (Section 6.2, item 1).
8. `rollback.yml`: automatic trigger on health-check failure + manual
   `workflow_dispatch` trigger.
9. Rollback test (Section 6.2, item 3) — the project's final milestone.
10. Documentation + benchmarks + final `PROJECT_STATUS.md` update marking
    the full 10-phase build complete.

Work through these one at a time. After each numbered step, report status
before moving to the next, per the working agreement in `GEMINI.md`.
