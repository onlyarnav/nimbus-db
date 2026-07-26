package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/onlyarnav/nimbusdb/services/auth-service/auth"
	"github.com/onlyarnav/nimbusdb/services/observability/telemetry"
)

type TokenRequest struct {
	UserID   string `json:"userId"`
	Role     string `json:"role"`
	Password string `json:"password,omitempty"`
}

type APIKeyRequest struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	slog.Info("starting nimbusdb auth-service microservice")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8087"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mux := http.NewServeMux()

	// Public token endpoint for local login/token issuance
	mux.HandleFunc("POST /v1/auth/token", func(w http.ResponseWriter, r *http.Request) {
		var req TokenRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.UserID == "" {
			req.UserID = "demo-user"
		}
		if req.Role == "" {
			req.Role = auth.RoleReadOnly
		}

		token, err := auth.IssueToken(req.UserID, req.Role, 24*time.Hour)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"token":     token,
			"tokenType": "Bearer",
			"expiresIn": 86400,
			"role":      req.Role,
		})
	})

	// Admin API key creation endpoint
	mux.HandleFunc("POST /v1/auth/api-keys", func(w http.ResponseWriter, r *http.Request) {
		var req APIKeyRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Name == "" {
			req.Name = "service-key"
		}
		if req.Role == "" {
			req.Role = auth.RoleOperator
		}

		keyID, rawKey, err := auth.GetGlobalKeyStore().CreateAPIKey(req.Name, req.Role)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"keyId":  keyID,
			"apiKey": rawKey,
			"role":   req.Role,
			"note":   "Save this API key; raw key is never stored!",
		})
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "UP", "service": "auth-service"})
	})
	mux.Handle("/metrics", telemetry.MetricsHandler())

	server := &http.Server{
		Addr:    fmt.Sprintf(":%s", port),
		Handler: mux,
	}

	go func() {
		slog.Info("auth-service HTTP listening", "port", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("auth-service failed", "error", err)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down auth-service gracefully")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
}
