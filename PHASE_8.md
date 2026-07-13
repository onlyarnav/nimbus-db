# PHASE_8.md — Security

Companion spec to `GEMINI.md`, `PHASE_1.md`–`PHASE_7.md`. Do not start
this phase until Phase 7's acceptance criteria are fully met (see
`PROJECT_STATUS.md`). This phase assumes a working multi-region cluster
with full observability, vector search, and cloud-operations tooling
already in place.

**Important framing note:** every prior phase explicitly deferred auth
("APIs remain unauthenticated in dev, known gap, not an oversight" — see
`GEMINI.md` and every `PHASE_N.md` Non-Goals section). This phase is where
that debt gets paid off across the *entire* system, not just new
endpoints. Section 2 below requires an audit of every existing
client-facing and internal endpoint before adding new security features.

**Protocol note:** Auth tokens/credentials are propagated via gRPC
metadata for internal calls and standard HTTP headers (`Authorization:
Bearer ...`) at the REST edge — consistent with the existing gRPC-internal/
REST-edge split established since Phase 1. Do not introduce a third
protocol or a parallel auth mechanism for internal vs external calls;
the token format should be the same, just carried differently per
transport.

---

## 1. Goal of This Phase

Add enterprise-grade security to the whole system: OAuth-based
authentication, real role-based access control (not a decorative field),
API keys for service/programmatic access, audit logging, encryption at
rest and in transit, secrets management, and rate limiting.

**Definition of done:** every previously-unauthenticated endpoint (Gateway
REST edge, all internal gRPC calls) now requires valid authentication;
RBAC actually denies requests from insufficiently-privileged roles,
proven by a test that expects and gets a rejection; sensitive data is
encrypted at rest; secrets are never hardcoded or committed; rate limiting
is enforced and testable.

---

## 2. Pre-Work: Endpoint Audit (do this first, before any new code)

List every endpoint across every service built in Phases 1-7:
- Gateway REST endpoints (`/v1/databases`, `/v1/regions`, etc.)
- Every internal gRPC service (Metadata Service, Scheduler, Control Plane,
  Node Agent, Deployment Controller, Capacity Planner, SLA Monitor)
- Dashboard's data-fetching endpoints

Produce this list as `docs/security-audit.md` before writing any auth
code. This becomes the checklist Section 9 verifies against — do not skip
straight to building OAuth without this inventory, or endpoints will get
missed silently.

---

## 3. Services Touched in This Phase

```
services/
├── auth-service/     ← NEW: OAuth provider integration, token issuance/validation, API key management
├── gateway/             ← EXTENDED: auth middleware on every REST endpoint
├── (all internal services) ← EXTENDED: gRPC interceptor validating tokens on every call
└── secrets/                    ← NEW: secrets management approach (see Section 7)
```

`auth-service` centralizes token issuance and validation logic so every
other service doesn't reimplement it — services call into it (or use a
shared validation library backed by the same signing key/JWKS) rather than
each rolling their own auth check.

---

## 4. OAuth

### 4.1 Design
- Use an existing OAuth provider/library rather than implementing the
  OAuth spec from scratch (e.g., a self-hosted lightweight OAuth server
  like `ory/hydra`, or a simpler JWT-based bearer-token scheme if full
  OAuth2 flows are disproportionate scope — decide and document in
  `docs/decisions/auth-approach.md`).
- **Recommendation:** for a portfolio project at this scope, a JWT-based
  bearer token issued by `auth-service` after a simple credential check is
  a defensible, honest scope boundary — full OAuth2 authorization-code
  flow with a real identity provider is more infrastructure than this
  project needs to demonstrate the underlying access-control concepts.
  State clearly in the README which was chosen and why, so it doesn't read
  as an oversight.

### 4.2 Acceptance
- A request without a valid token is rejected (401) at both the Gateway
  REST edge and internal gRPC calls.
- A request with an expired/tampered token is rejected.

---

## 5. RBAC

### 5.1 Design (this is the part that must be real, per GEMINI.md
Section 5's acceptance criterion — no decorative role field)
- Define concrete roles for this system, e.g.: `admin` (full access,
  including deployment/scaling operations from Phase 7), `operator`
  (database CRUD, read observability data), `read-only` (queries only, no
  writes, no ops actions).
- Every endpoint declares which role(s) it requires.
- Enforcement happens at the auth middleware/interceptor layer (Section
  3), checking the token's embedded role claim against the endpoint's
  requirement — reject with 403 (REST) or `PERMISSION_DENIED` (gRPC) if
  insufficient.

### 5.2 Acceptance (explicit, matches GEMINI.md's stated requirement)
- A `read-only`-role token attempting a write operation (e.g.
  `POST /v1/databases`) is rejected — tested and proven, not asserted.
- A `read-only`-role token attempting a read operation succeeds.
- An `operator`-role token attempting a Phase 7 deployment/scaling
  operation is rejected (only `admin` can do that).

---

## 6. API Keys

### 6.1 Design
- For programmatic/service access (as distinct from human OAuth logins):
  long-lived API keys, associated with a role, revocable.
- Store only a hash of the key (never the raw key) — same principle as
  password storage; document this explicitly since it's a common security
  mistake to get wrong.

### 6.2 Acceptance
- A revoked API key is rejected on the next request after revocation
  (not just at issuance time — actually re-checked per-request or on a
  short cache TTL).

---

## 7. Secrets Management

### 7.1 Design
- No secrets (DB passwords, signing keys, API credentials) hardcoded or
  committed to the repo — enforce via `.gitignore` + a documented
  `.env.example` pattern, consistent with the 12-factor config convention
  already established in `GEMINI.md` Section 4.
- For this project's scope, environment-variable-based secrets loaded at
  startup is sufficient — a full HashiCorp Vault or Azure Key Vault
  integration is a reasonable stretch goal to *mention* as a "how this
  would scale to production" note, but not required for the working
  deliverable. Document this scope boundary explicitly.

### 7.2 Acceptance
- A repo scan (e.g. `git log -p | grep`-style check, or a tool like
  `gitleaks`) confirms no secrets were ever committed, including in
  earlier phases' history — run this as an actual check, not an assumption.

---

## 8. Encryption

### 8.1 In Transit
- TLS on the Gateway's REST edge (self-signed cert acceptable for local
  dev, document the production alternative).
- gRPC internal calls: TLS between services, or explicitly document that
  internal traffic is assumed to run on a trusted private network for
  this project's scope (a legitimate real-world pattern — many production
  systems terminate TLS at the edge and trust the internal network) —
  pick one and justify it, don't leave it unstated.

### 8.2 At Rest
- Sensitive fields (e.g., any PII-like data in the metadata store, API key
  hashes) encrypted at the application layer or via Postgres's native
  encryption features — pick one, document it.
- Storage engine (Phase 3) data files: full disk/page-level encryption is
  a legitimate stretch goal but likely disproportionate scope here;
  document as a known limitation with a note on how it would be added
  (e.g., encrypt at the page-manager layer before writing to disk) rather
  than silently skipping it.

---

## 9. Rate Limiting

### 9.1 Design
- Applied at the Gateway (REST edge) — per-token or per-API-key request
  rate limits, configurable per role (e.g., `admin` gets a higher limit
  than `read-only`).
- Standard approach: token bucket or sliding window, implemented with a
  library rather than from scratch, unless you specifically want the
  algorithm-implementation exercise (optional stretch, not required).

### 9.2 Acceptance
- Exceeding the configured rate limit results in a 429 response, and the
  limit correctly resets after the configured window — tested, not
  assumed.

---

## 10. Testing Requirements

### 10.1 Unit Tests
- Token validation: valid/expired/tampered tokens correctly
  accepted/rejected.
- RBAC middleware: correct allow/deny decisions across the role matrix
  defined in Section 5.1.
- API key hashing/revocation logic.
- Rate limiter: correct request counting and window reset behavior.

### 10.2 Integration Test (required — this is the phase's acceptance proof)
1. **Full endpoint audit re-check:** every endpoint listed in
   `docs/security-audit.md` (Section 2) is confirmed to now require valid
   auth — no endpoint silently missed.
2. **RBAC denial test (the core deliverable, matches GEMINI.md's explicit
   requirement):** a `read-only` token attempting a write is rejected; an
   `operator` token attempting an admin-only deployment operation is
   rejected. Both must produce an actual denial, verified by the test.
3. **API key revocation test:** issue, use successfully, revoke, confirm
   subsequent use fails.
4. **Rate limit test:** exceed the limit, confirm 429, confirm reset after
   the window.
5. **Secrets scan:** run the check from Section 7.2 against the full repo
   history, confirm clean.

### 10.3 What "done" looks like
- [ ] All unit tests pass
- [ ] All 5 integration scenarios in Section 10.2 pass
- [ ] `docs/security-audit.md` shows every endpoint from Phases 1-7 as
      now-authenticated, with none missed
- [ ] `docs/decisions/auth-approach.md`, encryption-in-transit approach,
      and any other Section 4-9 decisions logged
- [ ] `docs/benchmarks.md` updated with: auth overhead (latency added by
      token validation, measured before/after), rate limiter accuracy
- [ ] README for `auth-service` covering roles, token format, how to issue
      test tokens for local development
- [ ] `PROJECT_STATUS.md` updated to mark Phase 8 complete — only once
      every item above is actually done

---

## 11. Explicit Non-Goals for Phase 8

- Full OAuth2 authorization-code flow with a real external identity
  provider (JWT bearer-token scheme is the documented scope boundary,
  Section 4.1)
- Vault/Key Vault-grade secrets infrastructure (env-var-based secrets is
  the documented scope boundary, Section 7.1)
- Page-level storage encryption at rest (documented as a known limitation
  with a stated extension path, Section 8.2)
- Legal-hold / retention-lock semantics (this was raised earlier as a
  Rubrik-alignment suggestion and is explicitly parked per
  `PROJECT_STATUS.md` Section 4 — do not pull it into this phase's RBAC/
  audit-log work even though the surface area is superficially similar)

---

## 12. Suggested Build Order (within this phase)

1. Endpoint audit (Section 2) — `docs/security-audit.md`. Do this before
   any code.
2. Auth approach decision logged (Section 4.1 — gate before building).
3. `auth-service`: token issuance, validation, JWT signing/verification.
4. Gateway REST middleware: reject unauthenticated requests.
5. gRPC interceptor: reject unauthenticated internal calls — apply across
   every existing service from Phases 1-7 per the audit list.
6. RBAC role definitions + enforcement (Section 5) + the RBAC denial test
   (Section 10.2, item 2) — this is the phase's core deliverable.
7. API keys (Section 6) + revocation test.
8. Secrets audit/cleanup (Section 7) + repo scan.
9. TLS setup (Section 8.1) + at-rest encryption decisions (Section 8.2).
10. Rate limiting (Section 9) + test.
11. Full endpoint audit re-check (Section 10.2, item 1) — confirm nothing
    from the Section 2 inventory was missed.
12. Benchmarks + decision docs + README + `PROJECT_STATUS.md` update.

Work through these one at a time. After each numbered step, report status
before moving to the next, per the working agreement in `GEMINI.md`.
