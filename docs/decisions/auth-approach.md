# Architectural Decision Record: Authentication Scheme & Token Format Choice

## Context
In Phase 8, NimbusDB establishes enterprise-grade authentication and access control across all REST edge endpoints and internal gRPC services. We must choose between integrating a full external OAuth2 authorization-code server (e.g. Ory Hydra / Keycloak) and building a native, lightweight JWT (JSON Web Token) bearer authentication scheme issued by `services/auth-service`.

## Recommendation & Decision
NimbusDB adopts a **JWT Bearer Token authentication scheme** issued natively by `services/auth-service`.

## Rationale
1. **Honest Scope Boundary**: Full OAuth2 authorization-code flow with external identity providers (IDPs), redirect URIs, and browser consent screens is heavy external infrastructure that does not demonstrate distributed database systems engineering. A native JWT bearer token scheme cleanly demonstrates token signing, claims verification, role-based authorization (RBAC), and gRPC metadata context propagation without external IDP dependencies.
2. **Standardized Claims Structure**:
   - `sub`: User/Subject ID
   - `role`: Role claim (`admin`, `operator`, `read-only`)
   - `exp`: Expiration timestamp (HMAC-SHA256 signed using a 256-bit secret)
3. **Transport Consistency**:
   - REST Edge: Standard HTTP header `Authorization: Bearer <jwt-token>`
   - Internal gRPC: gRPC metadata `authorization: Bearer <jwt-token>`
