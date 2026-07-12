package tests

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/onlyarnav/nimbusdb/services/metadata-service/handlers"
)

func TestHealthHandler_Unit(t *testing.T) {
	tests := []struct {
		name           string
		pool           *pgxpool.Pool
		expectedStatus int
		expectedBody   handlers.HealthResponse
	}{
		{
			name:           "Nil pool returns 503 DOWN",
			pool:           nil,
			expectedStatus: http.StatusServiceUnavailable,
			expectedBody: handlers.HealthResponse{
				Status:   "DOWN",
				Database: "disconnected",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "/health", nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			rr := httptest.NewRecorder()
			handler := handlers.HealthHandler(tc.pool)

			handler.ServeHTTP(rr, req)

			if rr.Code != tc.expectedStatus {
				t.Errorf("expected status %d, got %d", tc.expectedStatus, rr.Code)
			}

			var body handlers.HealthResponse
			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}

			if body.Status != tc.expectedBody.Status {
				t.Errorf("expected response status %s, got %s", tc.expectedBody.Status, body.Status)
			}
			if body.Database != tc.expectedBody.Database {
				t.Errorf("expected response database %s, got %s", tc.expectedBody.Database, body.Database)
			}
		})
	}
}

func TestDatabaseMigrationAndSchema(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/nimbusdb?sslmode=disable"
	}

	// First verify if we can connect to PG
	testDB, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Skipf("Skipping database migration tests: unable to connect to test postgres: %v", err)
		return
	}
	if err := testDB.Ping(); err != nil {
		testDB.Close()
		t.Skipf("Skipping database migration tests: unable to ping test postgres: %v", err)
		return
	}
	testDB.Close()

	// Initialize migrate with file driver reading from ../migrations
	m, err := migrate.New("file://../migrations", dbURL)
	if err != nil {
		t.Fatalf("failed to initialize migrate: %v", err)
	}

	// 1. Rollback any existing schema (clean state)
	if err := m.Down(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("failed to roll back existing migrations: %v", err)
	}

	// 2. Run migrations Up
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	// Open database connection to verify table structure
	conn, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Fatalf("failed to open db for verification: %v", err)
	}
	defer conn.Close()

	// 3. Verify tables exist
	expectedTables := []string{"regions", "clusters", "nodes", "heartbeats", "databases", "replicas"}
	for _, table := range expectedTables {
		var exists bool
		query := `SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name = $1
		);`
		if err := conn.QueryRow(query, table).Scan(&exists); err != nil {
			t.Fatalf("failed to check existence of table %s: %v", table, err)
		}
		if !exists {
			t.Errorf("table %s was not created by migration", table)
		}
	}

	// 4. Test unique and foreign key constraints on regions/clusters
	// Insert region
	var regionID string
	err = conn.QueryRow("INSERT INTO regions (name) VALUES ('us-east') RETURNING id").Scan(&regionID)
	if err != nil {
		t.Fatalf("failed to insert region: %v", err)
	}

	// Verify region name uniqueness constraint
	_, err = conn.Exec("INSERT INTO regions (name) VALUES ('us-east')")
	if err == nil {
		t.Error("expected error due to duplicate region name, but got nil")
	}

	// Insert cluster
	var clusterID string
	err = conn.QueryRow("INSERT INTO clusters (name, region_id) VALUES ('cluster-1', $1) RETURNING id", regionID).Scan(&clusterID)
	if err != nil {
		t.Fatalf("failed to insert cluster: %v", err)
	}

	// Verify cluster name uniqueness constraint
	_, err = conn.Exec("INSERT INTO clusters (name, region_id) VALUES ('cluster-1', $1)", regionID)
	if err == nil {
		t.Error("expected error due to duplicate cluster name, but got nil")
	}

	// Insert node
	var nodeID string
	err = conn.QueryRow("INSERT INTO nodes (cluster_id, hostname) VALUES ($1, 'worker-1') RETURNING id", clusterID).Scan(&nodeID)
	if err != nil {
		t.Fatalf("failed to insert node: %v", err)
	}

	// Verify multi-column UNIQUE constraint (cluster_id, hostname)
	_, err = conn.Exec("INSERT INTO nodes (cluster_id, hostname) VALUES ($1, 'worker-1')", clusterID)
	if err == nil {
		t.Error("expected error due to duplicate (cluster_id, hostname), but got nil")
	}

	// 5. Test health handler with a real database pool
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to create pgxpool: %v", err)
	}
	defer pool.Close()

	req, err := http.NewRequest("GET", "/health", nil)
	if err != nil {
		t.Fatalf("failed to create health check request: %v", err)
	}
	rr := httptest.NewRecorder()
	handler := handlers.HealthHandler(pool)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var healthRes handlers.HealthResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &healthRes); err != nil {
		t.Fatalf("failed to unmarshal health check response: %v", err)
	}

	if healthRes.Status != "UP" || healthRes.Database != "connected" {
		t.Errorf("expected health check response status=UP, database=connected; got status=%s, database=%s", healthRes.Status, healthRes.Database)
	}

	// 6. Clean up by rolling back migrations
	if err := m.Down(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("failed to roll back migrations: %v", err)
	}

	// 7. Verify tables are dropped
	for _, table := range expectedTables {
		var exists bool
		query := `SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name = $1
		);`
		if err := conn.QueryRow(query, table).Scan(&exists); err != nil {
			t.Fatalf("failed to check table %s after rollback: %v", table, err)
		}
		if exists {
			t.Errorf("table %s was not dropped by rollback", table)
		}
	}
}
