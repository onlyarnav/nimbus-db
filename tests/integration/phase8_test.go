package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/onlyarnav/nimbusdb/services/auth-service/auth"
	gateway "github.com/onlyarnav/nimbusdb/services/gateway/handlers"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Scenario 1: Full Endpoint Audit Re-check
func TestPhase8_EndpointAuditReCheck(t *testing.T) {
	gw := gateway.NewGatewayHandlers(nil)
	mux := http.NewServeMux()
	gw.RegisterRoutes(mux)

	protectedRoutes := []struct {
		method string
		path   string
	}{
		{"POST", "/v1/databases"},
		{"GET", "/v1/databases/db-123"},
		{"GET", "/v1/databases"},
		{"DELETE", "/v1/databases/db-123"},
		{"GET", "/v1/regions"},
		{"POST", "/v1/deployments/rolling"},
		{"POST", "/v1/deployments/canary"},
		{"POST", "/v1/deployments/blue-green"},
		{"POST", "/v1/deployments/rollback"},
		{"POST", "/v1/nodes/node-1/drain"},
		{"GET", "/v1/capacity/projection"},
		{"GET", "/v1/sla/report"},
	}

	for _, r := range protectedRoutes {
		req := httptest.NewRequest(r.method, r.path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected route %s %s without auth to return HTTP 401 Unauthorized, got %d", r.method, r.path, rec.Code)
		}
	}
}

// Scenario 2: RBAC Denial Test (Read-only write rejection & Operator admin action rejection)
func TestPhase8_RBACDenialMatrix(t *testing.T) {
	readOnlyToken, err := auth.IssueToken("test-user-ro", auth.RoleReadOnly, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to issue read-only token: %v", err)
	}

	operatorToken, err := auth.IssueToken("test-user-op", auth.RoleOperator, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to issue operator token: %v", err)
	}

	gw := gateway.NewGatewayHandlers(nil)
	mux := http.NewServeMux()
	gw.RegisterRoutes(mux)

	// 1. Read-only user attempting write (POST /v1/databases) must be rejected with 403 Forbidden
	req1 := httptest.NewRequest("POST", "/v1/databases", strings.NewReader(`{"name":"db1","clusterId":"c1"}`))
	req1.Header.Set("Authorization", "Bearer "+readOnlyToken)
	rec1 := httptest.NewRecorder()
	mux.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusForbidden {
		t.Errorf("expected read-only write attempt to return 403 Forbidden, got %d", rec1.Code)
	}

	// 2. Operator user attempting admin deployment (POST /v1/deployments/rolling) must be rejected with 403 Forbidden
	req2 := httptest.NewRequest("POST", "/v1/deployments/rolling", strings.NewReader(`{"targetVersion":"v2"}`))
	req2.Header.Set("Authorization", "Bearer "+operatorToken)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusForbidden {
		t.Errorf("expected operator deployment attempt to return 403 Forbidden, got %d", rec2.Code)
	}

	// 3. gRPC Interceptor Denial Test
	roleMap := map[string]string{
		"/proto.MetadataService/UpdateNodeStatus": auth.RoleAdmin,
	}
	interceptor := auth.UnaryServerInterceptor(roleMap)

	dummyHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	}
	info := &grpc.UnaryServerInfo{FullMethod: "/proto.MetadataService/UpdateNodeStatus"}

	// Unauthenticated gRPC call must fail with Unauthenticated
	_, err = interceptor(context.Background(), nil, info, dummyHandler)
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("expected gRPC call without auth metadata to return Unauthenticated, got %v", err)
	}
}

// Scenario 3: API Key Revocation Test
func TestPhase8_APIKeyRevocation(t *testing.T) {
	store := auth.GetGlobalKeyStore()
	keyID, rawKey, err := store.CreateAPIKey("service-integration-key", auth.RoleOperator)
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}

	// Active key verification must succeed
	rec, err := store.VerifyAPIKey(rawKey)
	if err != nil || rec.Role != auth.RoleOperator {
		t.Fatalf("expected valid active key verification, got rec=%v err=%v", rec, err)
	}

	// Revoke key
	if err := store.RevokeAPIKey(keyID); err != nil {
		t.Fatalf("failed to revoke API key: %v", err)
	}

	// Verification immediately post-revocation must fail
	_, err = store.VerifyAPIKey(rawKey)
	if err != auth.ErrRevokedAPIKey {
		t.Errorf("expected ErrRevokedAPIKey post-revocation, got %v", err)
	}
}

// Scenario 4: Rate Limit Test
func TestPhase8_RateLimitEnforcement(t *testing.T) {
	customRules := map[string]auth.RateLimitRule{
		"test-role": {MaxRequests: 3, Window: 100 * time.Millisecond},
	}
	limiter := auth.NewRateLimiter(customRules)

	clientID := "rate-test-client"

	// 3 allowed requests
	for i := 0; i < 3; i++ {
		if !limiter.Allow(clientID, "test-role") {
			t.Errorf("request %d should have been allowed", i+1)
		}
	}

	// 4th request within window must be rate-limited
	if limiter.Allow(clientID, "test-role") {
		t.Errorf("request 4 should have been blocked by rate limiter")
	}

	// Wait for window reset
	time.Sleep(120 * time.Millisecond)

	// Post-reset request must succeed
	if !limiter.Allow(clientID, "test-role") {
		t.Errorf("request after window reset should have succeeded")
	}
}

// Scenario 5: Secrets Scan Test (git log history scan for exposed passwords/keys)
func TestPhase8_GitLogSecretsScan(t *testing.T) {
	cmd := exec.Command("git", "log", "-p", "-n", "50")
	cmd.Dir = "../.."
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("git command not available: %v", err)
	}

	content := string(out)
	suspiciousPatterns := []string{
		"AWS_SECRET_ACCESS_KEY=",
		"BEGIN PRIVATE KEY",
		"ghp_",
	}

	for _, pattern := range suspiciousPatterns {
		if strings.Contains(content, pattern) {
			t.Errorf("git log history contains exposed secret pattern %q", pattern)
		}
	}
}
