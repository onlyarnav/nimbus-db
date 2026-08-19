package auth

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/onlyarnav/nimbusdb/services/observability/telemetry"
)

type contextKey string

const ClaimsContextKey contextKey = "auth_claims"

// RoleSatisfies returns true if presentedRole meets or exceeds requiredRole in hierarchy.
func RoleSatisfies(presentedRole, requiredRole string) bool {
	if presentedRole == RoleAdmin {
		return true
	}
	if presentedRole == RoleOperator && (requiredRole == RoleOperator || requiredRole == RoleReadOnly) {
		return true
	}
	if presentedRole == RoleReadOnly && requiredRole == RoleReadOnly {
		return true
	}
	return false
}

// AuthenticateAndAuthorize creates HTTP middleware enforcing token/key validity, role authorization, and rate limiting.
func AuthenticateAndAuthorize(requiredRole string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Exempt /health and /metrics from authentication
			if r.URL.Path == "/health" || r.URL.Path == "/metrics" || r.URL.Path == "/v1/auth/token" {
				next.ServeHTTP(w, r)
				return
			}

			var presentedRole string
			var clientID string

			// 1. Check Bearer Token in Authorization header
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
				claims, err := VerifyToken(tokenStr)
				if err != nil {
					slog.WarnContext(ctx, "REST request rejected: invalid or expired bearer token", "error", err)
					http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
					return
				}
				presentedRole = claims.Role
				clientID = claims.Sub
				ctx = context.WithValue(ctx, ClaimsContextKey, claims)
			} else if apiKey := r.Header.Get("X-API-Key"); apiKey != "" { // 2. Check API Key
				rec, err := GetGlobalKeyStore().VerifyAPIKey(apiKey)
				if err != nil {
					slog.WarnContext(ctx, "REST request rejected: invalid or revoked API key", "error", err)
					http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
					return
				}
				presentedRole = rec.Role
				clientID = rec.ID
			} else {
				slog.WarnContext(ctx, "REST request rejected: missing authorization credentials", "path", r.URL.Path)
				http.Error(w, "Unauthorized: missing authorization credentials", http.StatusUnauthorized)
				return
			}

			// 3. Enforce Rate Limiting
			if !GetGlobalRateLimiter().Allow(clientID, presentedRole) {
				slog.WarnContext(ctx, "REST request rate limit exceeded", "client_id", clientID, "role", presentedRole)
				http.Error(w, "Too Many Requests: rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			// 4. Enforce RBAC Role Check
			if !RoleSatisfies(presentedRole, requiredRole) {
				slog.WarnContext(ctx, "REST request forbidden: insufficient role permissions", "presented_role", presentedRole, "required_role", requiredRole)
				telemetry.ErrorsTotal.WithLabelValues("auth", "rbac", "forbidden").Inc()
				http.Error(w, "Forbidden: insufficient permissions for role "+presentedRole, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UnaryServerInterceptor creates a gRPC server interceptor validating tokens/keys and enforcing RBAC.
func UnaryServerInterceptor(roleMap map[string]string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		requiredRole, requiresAuth := roleMap[info.FullMethod]
		if !requiresAuth {
			requiredRole = RoleReadOnly // default required role for internal RPCs
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing gRPC metadata context")
		}

		var presentedRole string
		var clientID string

		// 1. Check gRPC metadata authorization header
		authVals := md.Get("authorization")
		if len(authVals) > 0 && strings.HasPrefix(authVals[0], "Bearer ") {
			tokenStr := strings.TrimPrefix(authVals[0], "Bearer ")
			claims, err := VerifyToken(tokenStr)
			if err != nil {
				return nil, status.Errorf(codes.Unauthenticated, "invalid token: %v", err)
			}
			presentedRole = claims.Role
			clientID = claims.Sub
		} else if apiKeys := md.Get("x-api-key"); len(apiKeys) > 0 {
			rec, err := GetGlobalKeyStore().VerifyAPIKey(apiKeys[0])
			if err != nil {
				return nil, status.Errorf(codes.Unauthenticated, "invalid API key: %v", err)
			}
			presentedRole = rec.Role
			clientID = rec.ID
		} else {
			return nil, status.Error(codes.Unauthenticated, "missing authorization credentials in gRPC metadata")
		}

		// 2. Enforce Rate Limiting
		if !GetGlobalRateLimiter().Allow(clientID, presentedRole) {
			return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
		}

		// 3. Enforce RBAC Role Check
		if !RoleSatisfies(presentedRole, requiredRole) {
			slog.WarnContext(ctx, "gRPC RPC forbidden: insufficient role permissions", "method", info.FullMethod, "presented_role", presentedRole, "required_role", requiredRole)
			return nil, status.Errorf(codes.PermissionDenied, "permission denied: role %s cannot execute %s", presentedRole, info.FullMethod)
		}

		return handler(ctx, req)
	}
}

// UnaryClientInterceptor attaches a short-lived bearer token to internal gRPC
// calls. Services must still set JWT_SECRET; no client-side default is used.
func UnaryClientInterceptor(serviceID, role string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		token, err := IssueToken(serviceID, role, 5*time.Minute)
		if err != nil {
			return status.Errorf(codes.Unauthenticated, "issue internal service token: %v", err)
		}
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
