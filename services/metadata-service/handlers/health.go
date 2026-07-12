package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// HealthResponse defines the structure of the /health response.
type HealthResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
}

// HealthHandler returns the health of the service and its database connection.
func HealthHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Use request context with timeout for checking the database connection
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		var dbStatus string
		var statusCode int

		if pool == nil {
			slog.WarnContext(ctx, "health check failed: database pool is nil")
			dbStatus = "disconnected"
			statusCode = http.StatusServiceUnavailable
		} else if err := pool.Ping(ctx); err != nil {
			slog.ErrorContext(ctx, "health check failed: database ping error", "error", err.Error())
			dbStatus = "disconnected"
			statusCode = http.StatusServiceUnavailable
		} else {
			dbStatus = "connected"
			statusCode = http.StatusOK
		}

		response := HealthResponse{
			Status:   "UP",
			Database: dbStatus,
		}
		if statusCode == http.StatusServiceUnavailable {
			response.Status = "DOWN"
		}

		w.WriteHeader(statusCode)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			slog.ErrorContext(ctx, "health check failed: failed to encode JSON response", "error", err.Error())
		}
	}
}
