# NimbusDB Platform Security Audit & Endpoint Inventory

This document lists every endpoint across all microservices built in Phases 1–7. In Phase 8, authentication and Role-Based Access Control (RBAC) middleware/interceptors are applied across this full inventory.

---

## 1. REST Edge Endpoints (`Gateway` & `Metadata Service`)

| Endpoint Path | HTTP Method | Target Service | Minimum Required Role | Auth Enforcement Status |
|---------------|-------------|----------------|-----------------------|-------------------------|
| `/v1/auth/token` | `POST` | `auth-service` | **Public** (Issuer) | ✅ Unauthenticated Login |
| `/v1/auth/api-keys` | `POST` | `auth-service` | `admin` | ✅ Authenticated (Bearer/Key) |
| `/v1/databases` | `POST` | `Gateway` / `Control Plane` | `operator` | ✅ Authenticated & Enforced |
| `/v1/databases/{id}` | `GET` | `Gateway` / `Control Plane` | `read-only` | ✅ Authenticated & Enforced |
| `/v1/databases` | `GET` | `Gateway` / `Control Plane` | `read-only` | ✅ Authenticated & Enforced |
| `/v1/databases/{id}` | `DELETE` | `Gateway` / `Control Plane` | `operator` | ✅ Authenticated & Enforced |
| `/v1/regions` | `GET` | `Gateway` | `read-only` | ✅ Authenticated & Enforced |
| `/v1/nodes` | `GET` | `Metadata Service` | `read-only` | ✅ Authenticated & Enforced |
| `/v1/deployments/rolling` | `POST` | `Gateway` | `admin` | ✅ Authenticated & Enforced |
| `/v1/deployments/canary` | `POST` | `Gateway` | `admin` | ✅ Authenticated & Enforced |
| `/v1/deployments/blue-green` | `POST` | `Gateway` | `admin` | ✅ Authenticated & Enforced |
| `/v1/deployments/rollback` | `POST` | `Gateway` | `admin` | ✅ Authenticated & Enforced |
| `/v1/nodes/{id}/drain` | `POST` | `Gateway` | `admin` | ✅ Authenticated & Enforced |
| `/v1/capacity/projection` | `GET` | `Gateway` | `read-only` | ✅ Authenticated & Enforced |
| `/v1/sla/report` | `GET` | `Gateway` | `read-only` | ✅ Authenticated & Enforced |
| `/health` | `GET` | All Services | **Exempt** (Health Check) | ✅ Exempt |
| `/metrics` | `GET` | All Services | **Exempt** (Prometheus Scrape) | ✅ Exempt |

---

## 2. Internal gRPC Services

### 2.1 Metadata Service (`port 50051`)
| RPC Method | Minimum Required Role | Auth Enforcement Status |
|------------|-----------------------|-------------------------|
| `RegisterNode` | `operator` | ✅ gRPC Interceptor Enforced |
| `SendHeartbeat` | `operator` | ✅ gRPC Interceptor Enforced |
| `GetNodes` | `read-only` | ✅ gRPC Interceptor Enforced |
| `CreateDatabaseRecord` | `operator` | ✅ gRPC Interceptor Enforced |
| `UpdateDatabaseStatus` | `operator` | ✅ gRPC Interceptor Enforced |
| `GetDatabase` | `read-only` | ✅ gRPC Interceptor Enforced |
| `ListDatabases` | `read-only` | ✅ gRPC Interceptor Enforced |
| `DeleteDatabaseRecord` | `operator` | ✅ gRPC Interceptor Enforced |
| `UpdateNodeStatus` | `admin` | ✅ gRPC Interceptor Enforced |

### 2.2 Scheduler Service (`port 50052`)
| RPC Method | Minimum Required Role | Auth Enforcement Status |
|------------|-----------------------|-------------------------|
| `Schedule` | `operator` | ✅ gRPC Interceptor Enforced |

### 2.3 Node Agent / Worker Node (`port 50053`)
| RPC Method | Minimum Required Role | Auth Enforcement Status |
|------------|-----------------------|-------------------------|
| `CreateDatabase` | `operator` | ✅ gRPC Interceptor Enforced |
| `DeleteDatabase` | `operator` | ✅ gRPC Interceptor Enforced |
| `BackupDatabase` | `operator` | ✅ gRPC Interceptor Enforced |
| `RestoreDatabase` | `operator` | ✅ gRPC Interceptor Enforced |
| `InsertVector` | `operator` | ✅ gRPC Interceptor Enforced |
| `SearchVector` | `read-only` | ✅ gRPC Interceptor Enforced |
| `DrainNode` | `admin` | ✅ gRPC Interceptor Enforced |

### 2.4 Deployment Controller Service (`port 50058`)
| RPC Method | Minimum Required Role | Auth Enforcement Status |
|------------|-----------------------|-------------------------|
| `StartDeployment` | `admin` | ✅ gRPC Interceptor Enforced |
| `GetDeploymentStatus` | `read-only` | ✅ gRPC Interceptor Enforced |
| `TriggerRollback` | `admin` | ✅ gRPC Interceptor Enforced |
| `DrainNode` | `admin` | ✅ gRPC Interceptor Enforced |

### 2.5 Capacity Planner Service (`port 50059`)
| RPC Method | Minimum Required Role | Auth Enforcement Status |
|------------|-----------------------|-------------------------|
| `PredictCapacity` | `read-only` | ✅ gRPC Interceptor Enforced |

### 2.6 SLA Monitor Service (`port 50060`)
| RPC Method | Minimum Required Role | Auth Enforcement Status |
|------------|-----------------------|-------------------------|
| `RecordEvent` | `operator` | ✅ gRPC Interceptor Enforced |
| `GetSLAReport` | `read-only` | ✅ gRPC Interceptor Enforced |

---

## 3. Dashboard Data Fetching (`services/dashboard`)
| Data Target | Transport | Required Auth | Status |
|-------------|-----------|---------------|--------|
| `/v1/nodes` | HTTP GET | `Authorization: Bearer <token>` | ✅ Authenticated |
