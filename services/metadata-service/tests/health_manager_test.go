package tests

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/onlyarnav/nimbusdb/services/metadata-service/db"
)

func TestHealthManager_Classification(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:password@localhost:5432/nimbusdb?sslmode=disable"
	}

	// Connect and ping check
	testDB, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Skipf("Skipping integration: database connection failed: %v", err)
		return
	}
	if err := testDB.Ping(); err != nil {
		testDB.Close()
		t.Skipf("Skipping integration: database ping failed: %v", err)
		return
	}
	testDB.Close()

	// Apply migrations
	m, err := migrate.New("file://../migrations", dbURL)
	if err != nil {
		t.Fatalf("failed to initialize migrations: %v", err)
	}
	_ = m.Down()
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("failed to run migrations up: %v", err)
	}
	defer func() {
		_ = m.Down()
	}()

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to initialize pgx pool: %v", err)
	}
	defer pool.Close()

	// Connect for test state setup
	conn, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Fatalf("failed to connect standard sql driver: %v", err)
	}
	defer conn.Close()

	// Setup Region & Cluster
	var regionID string
	err = conn.QueryRow("INSERT INTO regions (name) VALUES ('test-region') RETURNING id").Scan(&regionID)
	if err != nil {
		t.Fatalf("failed to create region: %v", err)
	}
	var clusterID string
	err = conn.QueryRow("INSERT INTO clusters (name, region_id) VALUES ('test-cluster', $1) RETURNING id", regionID).Scan(&clusterID)
	if err != nil {
		t.Fatalf("failed to create cluster: %v", err)
	}

	// Mock node times
	tHealthy := time.Now().Add(-5 * time.Second)
	tUnhealthy := time.Now().Add(-20 * time.Second)
	tDead := time.Now().Add(-70 * time.Second)

	var node1, node2, node3, node4 string
	// Node 1: Healthy (heartbeat <15s ago)
	err = conn.QueryRow("INSERT INTO nodes (cluster_id, hostname, status, last_heartbeat) VALUES ($1, 'node-healthy', 'unknown', $2) RETURNING id", clusterID, tHealthy).Scan(&node1)
	if err != nil {
		t.Fatalf("failed to insert healthy node: %v", err)
	}
	// Node 2: Unhealthy (no heartbeat 15s to 60s ago)
	err = conn.QueryRow("INSERT INTO nodes (cluster_id, hostname, status, last_heartbeat) VALUES ($1, 'node-unhealthy', 'healthy', $2) RETURNING id", clusterID, tUnhealthy).Scan(&node2)
	if err != nil {
		t.Fatalf("failed to insert unhealthy node: %v", err)
	}
	// Node 3: Dead (no heartbeat >60s ago)
	err = conn.QueryRow("INSERT INTO nodes (cluster_id, hostname, status, last_heartbeat) VALUES ($1, 'node-dead', 'healthy', $2) RETURNING id", clusterID, tDead).Scan(&node3)
	if err != nil {
		t.Fatalf("failed to insert dead node: %v", err)
	}
	// Node 4: Overloaded (3 consecutive heartbeats >90%)
	err = conn.QueryRow("INSERT INTO nodes (cluster_id, hostname, status, last_heartbeat) VALUES ($1, 'node-overloaded', 'healthy', $2) RETURNING id", clusterID, tHealthy).Scan(&node4)
	if err != nil {
		t.Fatalf("failed to insert overloaded node: %v", err)
	}

	// Insert 3 overloaded heartbeats (CPU >90%) for Node 4
	for i := 0; i < 3; i++ {
		_, err = conn.Exec("INSERT INTO heartbeats (node_id, cpu_pct, memory_pct, disk_pct, healthy, received_at) VALUES ($1, 95.0, 10.0, 10.0, true, $2)", node4, time.Now().Add(-time.Duration(i)*5*time.Second))
		if err != nil {
			t.Fatalf("failed to insert overloaded heartbeat %d: %v", i, err)
		}
	}

	// Execute HealthManager check loop
	hm := db.NewHealthManager(pool, 1*time.Second)
	if err := hm.CheckHealth(ctx); err != nil {
		t.Fatalf("CheckHealth failed during evaluation: %v", err)
	}

	// Assert updated states in database
	var s1, s2, s3, s4 string
	err = conn.QueryRow("SELECT status FROM nodes WHERE id = $1", node1).Scan(&s1)
	if err != nil {
		t.Fatal(err)
	}
	err = conn.QueryRow("SELECT status FROM nodes WHERE id = $1", node2).Scan(&s2)
	if err != nil {
		t.Fatal(err)
	}
	err = conn.QueryRow("SELECT status FROM nodes WHERE id = $1", node3).Scan(&s3)
	if err != nil {
		t.Fatal(err)
	}
	err = conn.QueryRow("SELECT status FROM nodes WHERE id = $1", node4).Scan(&s4)
	if err != nil {
		t.Fatal(err)
	}

	if s1 != "healthy" {
		t.Errorf("expected node 1 to be classified 'healthy', got %q", s1)
	}
	if s2 != "unhealthy" {
		t.Errorf("expected node 2 to be classified 'unhealthy', got %q", s2)
	}
	if s3 != "dead" {
		t.Errorf("expected node 3 to be classified 'dead', got %q", s3)
	}
	if s4 != "overloaded" {
		t.Errorf("expected node 4 to be classified 'overloaded', got %q", s4)
	}
}
