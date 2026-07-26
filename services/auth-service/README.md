# NimbusDB Auth Service (`services/auth-service`)

## Overview
`auth-service` centralizes authentication, token signing, API key hashing, and Role-Based Access Control (RBAC) across the NimbusDB platform:
- **JWT Bearer Token Issuance**: HMAC-SHA256 signed bearer tokens with embedded `sub`, `role`, and `exp` claims.
- **API Key Management**: Generates long-lived programmatic keys (`nb_ak_...`), stores **only SHA-256 hashes** of keys, and supports instant revocation.
- **Role-Based Access Control (RBAC)**:
  - `admin`: Full cluster access, including Phase 7 deployment/scaling operations.
  - `operator`: Database CRUD operations and worker node management.
  - `read-only`: Read queries, observability telemetry, and capacity forecast reports.
- **Token Bucket Rate Limiting**: Per-client rate bounds (e.g. `100 req/min` for `read-only`, `1000 req/min` for `admin`).

## Token Issuance Local Dev Instructions
To issue a test JWT bearer token for local development:
```bash
curl -X POST http://localhost:8087/v1/auth/token \
  -H "Content-Type: application/json" \
  -d '{"userId": "demo-admin", "role": "admin"}'
```

## Known Scope Boundaries
- **Auth Scheme Choice**: Uses a native JWT bearer scheme rather than an external heavy OAuth2 server (logged in `docs/decisions/auth-approach.md`).
- **No Vault Infrastructure**: Secrets loaded via 12-factor environment variables (`.env.example`).
- **No Legal-Hold/Retention Locks**: Expressly excluded per project constitution (`PROJECT_STATUS.md`).
