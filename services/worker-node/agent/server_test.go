package agent

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/onlyarnav/nimbusdb/services/worker-node/proto/nodeagent"
)

const bufSize = 1024 * 1024

func TestNodeAgent(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	hostname := "test-worker-node"

	s := NewServer(dataDir, hostname)

	// Set up in-memory gRPC server
	lis := bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer()
	pb.RegisterNodeAgentServer(grpcServer, s)
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.GracefulStop()

	// Establish client connection
	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	client := pb.NewNodeAgentClient(conn)

	// 1. Happy Path CreateDatabase
	t.Run("CreateDatabaseRecord_Happy", func(t *testing.T) {
		res, err := client.CreateDatabase(ctx, &pb.CreateDatabaseRequest{
			Name:       "orders",
			DatabaseId: "00000000-0000-0000-0000-000000000001",
		})
		if err != nil {
			t.Fatalf("create database failed: %v", err)
		}
		if !res.GetSuccess() {
			t.Fatal("expected success to be true")
		}
		expectedEndpoint := "test-worker-node/db/00000000-0000-0000-0000-000000000001"
		if res.GetEndpoint() != expectedEndpoint {
			t.Errorf("expected endpoint %q, got %q", expectedEndpoint, res.GetEndpoint())
		}

		// Verify directory was created
		dbPath := filepath.Join(dataDir, "00000000-0000-0000-0000-000000000001")
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			t.Errorf("expected database directory %q to exist", dbPath)
		}
	})

	// 2. Reject Duplicate Name / ID
	t.Run("CreateDatabaseRecord_Duplicate", func(t *testing.T) {
		_, err := client.CreateDatabase(ctx, &pb.CreateDatabaseRequest{
			Name:       "orders",
			DatabaseId: "00000000-0000-0000-0000-000000000002",
		})
		if err == nil {
			t.Fatal("expected error on duplicate database name, got nil")
		}
		st, ok := status.FromError(err)
		if !ok || st.Code() != codes.AlreadyExists {
			t.Errorf("expected codes.AlreadyExists, got code: %v", st.Code())
		}

		_, err = client.CreateDatabase(ctx, &pb.CreateDatabaseRequest{
			Name:       "payments",
			DatabaseId: "00000000-0000-0000-0000-000000000001",
		})
		if err == nil {
			t.Fatal("expected error on duplicate database ID, got nil")
		}
	})

	// 3. Failure Injection
	t.Run("CreateDatabaseRecord_InjectedFailure", func(t *testing.T) {
		atomic.StoreInt32(&s.FailAttempts, 1)

		res, err := client.CreateDatabase(ctx, &pb.CreateDatabaseRequest{
			Name:       "failure-test",
			DatabaseId: "00000000-0000-0000-0000-000000000003",
		})
		if err != nil {
			t.Fatalf("gRPC call failed: %v", err)
		}
		if res.GetSuccess() {
			t.Fatal("expected success to be false due to injected failure")
		}

		// Subsequent call should succeed
		res, err = client.CreateDatabase(ctx, &pb.CreateDatabaseRequest{
			Name:       "failure-test",
			DatabaseId: "00000000-0000-0000-0000-000000000003",
		})
		if err != nil || !res.GetSuccess() {
			t.Fatalf("subsequent create database should have succeeded: err=%v, res=%v", err, res)
		}
	})

	// 4. Hang/Timeout Injection
	t.Run("CreateDatabaseRecord_InjectedHang", func(t *testing.T) {
		atomic.StoreInt32(&s.HangAttempts, 1)

		hangCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()

		_, err := client.CreateDatabase(hangCtx, &pb.CreateDatabaseRequest{
			Name:       "hang-test",
			DatabaseId: "00000000-0000-0000-0000-000000000004",
		})
		if err == nil {
			t.Fatal("expected context deadline error, got nil")
		}
		st, ok := status.FromError(err)
		if !ok || st.Code() != codes.DeadlineExceeded {
			t.Errorf("expected codes.DeadlineExceeded, got code: %v", st.Code())
		}
	})

	// 5. Delete Database
	t.Run("DeleteDatabaseRecord", func(t *testing.T) {
		res, err := client.DeleteDatabase(ctx, &pb.DeleteDatabaseRequest{
			DatabaseId: "00000000-0000-0000-0000-000000000001",
		})
		if err != nil {
			t.Fatalf("delete database failed: %v", err)
		}
		if !res.GetSuccess() {
			t.Fatal("expected success to be true")
		}

		// Verify directory is deleted
		dbPath := filepath.Join(dataDir, "00000000-0000-0000-0000-000000000001")
		if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
			t.Errorf("expected database directory %q to be deleted", dbPath)
		}
	})

	// 6. Backup/Restore Stubs (Unimplemented)
	t.Run("BackupRestoreStubs", func(t *testing.T) {
		_, err := client.BackupDatabase(ctx, &pb.BackupDatabaseRequest{DatabaseId: "xyz"})
		if err == nil {
			t.Fatal("expected unimplemented error for BackupDatabase")
		}
		st, ok := status.FromError(err)
		if !ok || st.Code() != codes.Unimplemented {
			t.Errorf("expected codes.Unimplemented, got: %v", st.Code())
		}

		_, err = client.RestoreDatabase(ctx, &pb.RestoreDatabaseRequest{DatabaseId: "xyz"})
		if err == nil {
			t.Fatal("expected unimplemented error for RestoreDatabase")
		}
	})
}
