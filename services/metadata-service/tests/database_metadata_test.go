package tests

import (
	"context"
	"database/sql"
	"net"
	"os"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	grpcserver "github.com/onlyarnav/nimbusdb/services/metadata-service/grpc"
	pb "github.com/onlyarnav/nimbusdb/services/metadata-service/proto"
)

func TestGRPCDatabaseMetadata(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:password@localhost:5432/nimbusdb?sslmode=disable"
	}

	// Connect to database to verify it is running
	testDB, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Skipf("Skipping integration tests: unable to connect to test postgres: %v", err)
		return
	}
	if err := testDB.Ping(); err != nil {
		testDB.Close()
		t.Skipf("Skipping integration tests: unable to ping test postgres: %v", err)
		return
	}
	testDB.Close()

	// Initialize schema migrations
	m, err := migrate.New("file://../migrations", dbURL)
	if err != nil {
		t.Fatalf("failed to initialize migrate: %v", err)
	}
	if err := m.Down(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("failed to roll back existing migrations: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("failed to apply migrations: %v", err)
	}
	defer func() {
		_ = m.Down()
	}()

	// Create connection pool
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	// Set up gRPC listener
	l := bufconn.Listen(bufSize)
	s := grpc.NewServer()
	pb.RegisterMetadataServiceServer(s, grpcserver.NewServer(pool))
	go func() {
		_ = s.Serve(l)
	}()
	defer s.GracefulStop()

	// Establish client connection
	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return l.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to dial bufnet: %v", err)
	}
	defer conn.Close()

	client := pb.NewMetadataServiceClient(conn)

	// Seed cluster and node
	var regionID string
	connDB, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Fatalf("failed to open raw DB conn: %v", err)
	}
	defer connDB.Close()

	err = connDB.QueryRow("INSERT INTO regions (name) VALUES ('metadata-test-region') RETURNING id").Scan(&regionID)
	if err != nil {
		t.Fatalf("failed to insert region: %v", err)
	}
	var clusterID string
	err = connDB.QueryRow("INSERT INTO clusters (name, region_id) VALUES ('metadata-test-cluster', $1) RETURNING id", regionID).Scan(&clusterID)
	if err != nil {
		t.Fatalf("failed to insert cluster: %v", err)
	}
	var nodeID string
	err = connDB.QueryRow("INSERT INTO nodes (cluster_id, hostname, status) VALUES ($1, 'test-host-1', 'healthy') RETURNING id", clusterID).Scan(&nodeID)
	if err != nil {
		t.Fatalf("failed to insert node: %v", err)
	}

	// 1. Create a database record
	t.Run("CreateDatabaseRecord", func(t *testing.T) {
		res, err := client.CreateDatabaseRecord(ctx, &pb.CreateDatabaseRecordRequest{
			Name:      "test-db",
			ClusterId: clusterID,
			NodeId:    nodeID,
			Status:    "provisioning",
			Attempts:  1,
		})
		if err != nil {
			t.Fatalf("failed to create database record: %v", err)
		}
		if res.GetDatabaseId() == "" {
			t.Fatal("expected non-empty database ID")
		}

		// Verify replica was created
		var role string
		err = connDB.QueryRow("SELECT role FROM replicas WHERE database_id = $1 AND node_id = $2", res.GetDatabaseId(), nodeID).Scan(&role)
		if err != nil {
			t.Fatalf("failed to query replica: %v", err)
		}
		if role != "leader" {
			t.Errorf("expected replica role to be 'leader', got %q", role)
		}

		// 2. Get the database record
		t.Run("GetDatabaseInfo", func(t *testing.T) {
			getRes, err := client.GetDatabase(ctx, &pb.GetDatabaseRequest{DatabaseId: res.GetDatabaseId()})
			if err != nil {
				t.Fatalf("failed to get database: %v", err)
			}
			db := getRes.GetDatabase()
			if db.GetName() != "test-db" {
				t.Errorf("expected name 'test-db', got %q", db.GetName())
			}
			if db.GetStatus() != "provisioning" {
				t.Errorf("expected status 'provisioning', got %q", db.GetStatus())
			}
			if db.GetNodeId() != nodeID {
				t.Errorf("expected node_id %q, got %q", nodeID, db.GetNodeId())
			}
		})

		// 3. Update database status
		t.Run("UpdateDatabaseStatus", func(t *testing.T) {
			// Seed another node for retry update
			var nodeID2 string
			err = connDB.QueryRow("INSERT INTO nodes (cluster_id, hostname, status) VALUES ($1, 'test-host-2', 'healthy') RETURNING id", clusterID).Scan(&nodeID2)
			if err != nil {
				t.Fatalf("failed to insert node2: %v", err)
			}

			upRes, err := client.UpdateDatabaseStatus(ctx, &pb.UpdateDatabaseStatusRequest{
				DatabaseId: res.GetDatabaseId(),
				Status:     "active",
				NodeId:     nodeID2,
				Endpoint:   "localhost:9000",
				Attempts:   2,
			})
			if err != nil {
				t.Fatalf("failed to update status: %v", err)
			}
			if !upRes.GetSuccess() {
				t.Fatal("expected update success to be true")
			}

			// Verify updated info
			getRes, err := client.GetDatabase(ctx, &pb.GetDatabaseRequest{DatabaseId: res.GetDatabaseId()})
			if err != nil {
				t.Fatalf("failed to get database post-update: %v", err)
			}
			db := getRes.GetDatabase()
			if db.GetStatus() != "active" {
				t.Errorf("expected status 'active', got %q", db.GetStatus())
			}
			if db.GetNodeId() != nodeID2 {
				t.Errorf("expected node_id %q, got %q", nodeID2, db.GetNodeId())
			}
			if db.GetEndpoint() != "localhost:9000" {
				t.Errorf("expected endpoint 'localhost:9000', got %q", db.GetEndpoint())
			}
			if db.GetAttempts() != 2 {
				t.Errorf("expected attempts 2, got %d", db.GetAttempts())
			}

			// Verify replicas table was updated (new leader at node2, old leader removed)
			var count int
			err = connDB.QueryRow("SELECT COUNT(*) FROM replicas WHERE database_id = $1", res.GetDatabaseId()).Scan(&count)
			if err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Errorf("expected exactly 1 replica, got %d", count)
			}

			var leaderNode string
			err = connDB.QueryRow("SELECT node_id FROM replicas WHERE database_id = $1 AND role = 'leader'", res.GetDatabaseId()).Scan(&leaderNode)
			if err != nil {
				t.Fatal(err)
			}
			if leaderNode != nodeID2 {
				t.Errorf("expected leader node to be %q, got %q", nodeID2, leaderNode)
			}
		})

		// 4. List databases
		t.Run("ListDatabases", func(t *testing.T) {
			listRes, err := client.ListDatabases(ctx, &pb.ListDatabasesRequest{})
			if err != nil {
				t.Fatalf("failed to list databases: %v", err)
			}
			if len(listRes.GetDatabases()) == 0 {
				t.Fatal("expected at least 1 database in list")
			}
			found := false
			for _, db := range listRes.GetDatabases() {
				if db.GetId() == res.GetDatabaseId() {
					found = true
				}
			}
			if !found {
				t.Fatal("created database not found in list")
			}
		})

		// 5. Delete database record
		t.Run("DeleteDatabaseRecord", func(t *testing.T) {
			delRes, err := client.DeleteDatabaseRecord(ctx, &pb.DeleteDatabaseRecordRequest{DatabaseId: res.GetDatabaseId()})
			if err != nil {
				t.Fatalf("failed to delete database: %v", err)
			}
			if !delRes.GetSuccess() {
				t.Fatal("expected delete success to be true")
			}

			// Verify database is removed
			_, err = client.GetDatabase(ctx, &pb.GetDatabaseRequest{DatabaseId: res.GetDatabaseId()})
			if err == nil {
				t.Fatal("expected error getting deleted database, but got nil")
			}

			// Verify replicas are removed
			var count int
			err = connDB.QueryRow("SELECT COUNT(*) FROM replicas WHERE database_id = $1", res.GetDatabaseId()).Scan(&count)
			if err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Errorf("expected 0 replicas after delete, got %d", count)
			}
		})
	})
}
