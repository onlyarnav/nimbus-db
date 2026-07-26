package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestJWTTokenIssuanceAndVerification(t *testing.T) {
	token, err := IssueToken("user-1", RoleAdmin, 1*time.Hour)
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}

	claims, err := VerifyToken(token)
	if err != nil {
		t.Fatalf("VerifyToken failed: %v", err)
	}

	if claims.Sub != "user-1" {
		t.Errorf("expected sub 'user-1', got %q", claims.Sub)
	}
	if claims.Role != RoleAdmin {
		t.Errorf("expected role 'admin', got %q", claims.Role)
	}
}

func TestExpiredJWTTokenRejection(t *testing.T) {
	// Issue token expired 5 seconds ago
	token, err := IssueToken("user-1", RoleOperator, -5*time.Second)
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}

	_, err = VerifyToken(token)
	if err != ErrExpiredToken {
		t.Errorf("expected ErrExpiredToken, got %v", err)
	}
}

func TestTamperedJWTTokenRejection(t *testing.T) {
	token, err := IssueToken("user-1", RoleAdmin, 1*time.Hour)
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}

	tampered := token + "tampered"
	_, err = VerifyToken(tampered)
	if err != ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestRBACRoleMatrixPermissions(t *testing.T) {
	if !RoleSatisfies(RoleAdmin, RoleAdmin) {
		t.Errorf("admin should satisfy admin")
	}
	if !RoleSatisfies(RoleAdmin, RoleOperator) {
		t.Errorf("admin should satisfy operator")
	}
	if !RoleSatisfies(RoleOperator, RoleReadOnly) {
		t.Errorf("operator should satisfy read-only")
	}
	if RoleSatisfies(RoleReadOnly, RoleOperator) {
		t.Errorf("read-only should NOT satisfy operator")
	}
	if RoleSatisfies(RoleOperator, RoleAdmin) {
		t.Errorf("operator should NOT satisfy admin")
	}
}

func TestAPIKeyHashingAndRevocation(t *testing.T) {
	store := NewAPIKeyStore()

	keyID, rawKey, err := store.CreateAPIKey("test-service", RoleOperator)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	rec, err := store.VerifyAPIKey(rawKey)
	if err != nil {
		t.Fatalf("VerifyAPIKey failed: %v", err)
	}
	if rec.Role != RoleOperator {
		t.Errorf("expected role 'operator', got %s", rec.Role)
	}

	// Revoke key
	if err := store.RevokeAPIKey(keyID); err != nil {
		t.Fatalf("RevokeAPIKey failed: %v", err)
	}

	// Immediate verification post-revocation should fail
	_, err = store.VerifyAPIKey(rawKey)
	if err != ErrRevokedAPIKey {
		t.Errorf("expected ErrRevokedAPIKey, got %v", err)
	}
}

func TestRateLimiter(t *testing.T) {
	rules := map[string]RateLimitRule{
		RoleReadOnly: {MaxRequests: 3, Window: 1 * time.Minute},
	}
	limiter := NewRateLimiter(rules)

	for i := 0; i < 3; i++ {
		if !limiter.Allow("client-1", RoleReadOnly) {
			t.Errorf("request %d should have been allowed", i+1)
		}
	}

	// 4th request should be rate-limited
	if limiter.Allow("client-1", RoleReadOnly) {
		t.Errorf("request 4 should have been rate limited")
	}
}

func TestRESTMiddlewareRBACDenial(t *testing.T) {
	readOnlyToken, _ := IssueToken("user-ro", RoleReadOnly, 1*time.Hour)

	// Handler expecting admin role
	adminHandler := AuthenticateAndAuthorize(RoleAdmin)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/v1/deployments/rolling", nil)
	req.Header.Set("Authorization", "Bearer "+readOnlyToken)
	rec := httptest.NewRecorder()

	adminHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected HTTP 403 Forbidden for read-only user on admin route, got %d", rec.Code)
	}
}
