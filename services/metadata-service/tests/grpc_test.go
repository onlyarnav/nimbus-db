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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	grpcserver "github.com/onlyarnav/nimbusdb/services/metadata-service/grpc"
	pb "github.com/onlyarnav/nimbusdb/services/metadata-service/proto"
)

const bufSize = 1024 * 1024

var lis *bufconn.Listener

func TestGRPCNodeRegistration(t *testing.T) {
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

	// Create connection pool
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	defer pool.Close()

	// Open standard database connection for setup insertions
	conn, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Fatalf("failed to open database for setup: %v", err)
	}
	defer conn.Close()

	// Setup gRPC server using bufconn (in-memory connection)
	lis = bufconn.Listen(bufSize)
	s := grpc.NewServer()
	pb.RegisterMetadataServiceServer(s, grpcserver.NewServer(pool))
	go func() {
		_ = s.Serve(lis)
	}()
	defer s.GracefulStop()

	// Dial the buffer listener
	connGrpc, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to dial bufnet: %v", err)
	}
	defer connGrpc.Close()

	client := pb.NewMetadataServiceClient(connGrpc)

	// Step 1: Register in a nonexistent cluster (should return codes.NotFound)
	_, err = client.RegisterNode(ctx, &pb.RegisterNodeRequest{
		ClusterId: "00000000-0000-0000-0000-000000000000",
		Hostname:  "worker-1",
	})
	if err == nil {
		t.Error("expected error when registering with nonexistent cluster, but got nil")
	} else {
		st, ok := status.FromError(err)
		if !ok {
			t.Errorf("expected gRPC status error, got: %v", err)
		} else if st.Code() != codes.NotFound {
			t.Errorf("expected codes.NotFound, got: %s", st.Code())
		}
	}

	// Insert a test region and a test cluster for verification
	var regionID string
	err = conn.QueryRow("INSERT INTO regions (name) VALUES ('us-west') RETURNING id").Scan(&regionID)
	if err != nil {
		t.Fatalf("failed to insert test region: %v", err)
	}

	var clusterID string
	err = conn.QueryRow("INSERT INTO clusters (name, region_id) VALUES ('cluster-test', $1) RETURNING id", regionID).Scan(&clusterID)
	if err != nil {
		t.Fatalf("failed to insert test cluster: %v", err)
	}

	// Step 2: Register successfully
	res, err := client.RegisterNode(ctx, &pb.RegisterNodeRequest{
		ClusterId: clusterID,
		Hostname:  "worker-1",
	})
	if err != nil {
		t.Fatalf("failed to register node: %v", err)
	}
	if len(res.GetNodeId()) == 0 {
		t.Error("expected returned nodeId to be non-empty")
	}
	if res.GetHeartbeatIntervalSeconds() != 5 {
		t.Errorf("expected heartbeatIntervalSeconds=5, got %d", res.GetHeartbeatIntervalSeconds())
	}

	// Step 3: Register duplicate hostname in the same cluster (should return codes.AlreadyExists)
	_, err = client.RegisterNode(ctx, &pb.RegisterNodeRequest{
		ClusterId: clusterID,
		Hostname:  "worker-1",
	})
	if err == nil {
		t.Error("expected error when registering duplicate hostname, but got nil")
	} else {
		st, ok := status.FromError(err)
		if !ok {
			t.Errorf("expected gRPC status error, got: %v", err)
		} else if st.Code() != codes.AlreadyExists {
			t.Errorf("expected codes.AlreadyExists, got: %s", st.Code())
		}
	}

	// Step 4: Input validation (empty parameters)
	_, err = client.RegisterNode(ctx, &pb.RegisterNodeRequest{
		ClusterId: "",
		Hostname:  "worker-1",
	})
	if err == nil {
		t.Error("expected error with empty cluster_id, but got nil")
	} else {
		st, ok := status.FromError(err)
		if !ok || st.Code() != codes.InvalidArgument {
			t.Errorf("expected codes.InvalidArgument, got: %v", err)
		}
	}

	_, err = client.RegisterNode(ctx, &pb.RegisterNodeRequest{
		ClusterId: clusterID,
		Hostname:  "",
	})
	if err == nil {
		t.Error("expected error with empty hostname, but got nil")
	} else {
		st, ok := status.FromError(err)
		if !ok || st.Code() != codes.InvalidArgument {
			t.Errorf("expected codes.InvalidArgument, got: %v", err)
		}
	}

	// Clean up by rolling back migrations
	if err := m.Down(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("failed to roll back migrations: %v", err)
	}
}
