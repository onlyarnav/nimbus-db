package tests

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	grpcserver "github.com/onlyarnav/nimbusdb/services/metadata-service/grpc"
	pb "github.com/onlyarnav/nimbusdb/services/metadata-service/proto"
)

func BenchmarkMetadataService(b *testing.B) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:password@localhost:5432/nimbusdb?sslmode=disable"
	}

	// Apply migrations
	m, err := migrate.New("file://../migrations", dbURL)
	if err != nil {
		b.Fatalf("failed to init migrate: %v", err)
	}
	_ = m.Down()
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		b.Fatalf("failed to apply migrations: %v", err)
	}
	defer func() {
		_ = m.Down()
	}()

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		b.Fatalf("failed to connect: %v", err)
	}
	defer pool.Close()

	// Clear table for clean benchmark
	conn, err := sql.Open("pgx", dbURL)
	if err != nil {
		b.Fatal(err)
	}
	defer conn.Close()

	var regionID string
	err = conn.QueryRow("INSERT INTO regions (name) VALUES ('bench-region') RETURNING id").Scan(&regionID)
	if err != nil {
		b.Fatal(err)
	}
	var clusterID string
	err = conn.QueryRow("INSERT INTO clusters (name, region_id) VALUES ('bench-cluster', $1) RETURNING id", regionID).Scan(&clusterID)
	if err != nil {
		b.Fatal(err)
	}
	conn.Close()

	// Setup server using bufconn (in-memory)
	lis = bufconn.Listen(bufSize)
	s := grpc.NewServer()
	pb.RegisterMetadataServiceServer(s, grpcserver.NewServer(pool))
	go func() {
		_ = s.Serve(lis)
	}()
	defer s.GracefulStop()

	// Dial in-memory server
	connGrpc, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		b.Fatal(err)
	}
	defer connGrpc.Close()
	client := pb.NewMetadataServiceClient(connGrpc)

	b.ResetTimer()

	// Measure RegisterNode Latency
	b.Run("RegisterNode", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			hostname := fmt.Sprintf("bench-node-%d-%d", b.N, i)
			_, err := client.RegisterNode(ctx, &pb.RegisterNodeRequest{
				ClusterId: clusterID,
				Hostname:  hostname,
			})
			if err != nil {
				b.Fatalf("RegisterNode failed: %v", err)
			}
		}
	})

	// Setup a node for SendHeartbeat
	regRes, err := client.RegisterNode(ctx, &pb.RegisterNodeRequest{
		ClusterId: clusterID,
		Hostname:  "heartbeat-bench-node",
	})
	if err != nil {
		b.Fatal(err)
	}
	nodeID := regRes.GetNodeId()

	// Measure SendHeartbeat Latency
	b.Run("SendHeartbeat", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := client.SendHeartbeat(ctx, &pb.SendHeartbeatRequest{
				NodeId:    nodeID,
				CpuPct:    45.0,
				MemoryPct: 55.0,
				DiskPct:   65.0,
				Healthy:   true,
			})
			if err != nil {
				b.Fatalf("SendHeartbeat failed: %v", err)
			}
		}
	})
}
