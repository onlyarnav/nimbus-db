package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"

	pb "github.com/onlyarnav/nimbusdb/services/control-plane/proto/metadata"
)

// Mock Metadata Client
type mockMetadataClient struct {
	pb.MetadataServiceClient
	createDbFn func(ctx context.Context, in *pb.CreateDatabaseRecordRequest, opts ...grpc.CallOption) (*pb.CreateDatabaseRecordResponse, error)
	updateDbFn func(ctx context.Context, in *pb.UpdateDatabaseStatusRequest, opts ...grpc.CallOption) (*pb.UpdateDatabaseStatusResponse, error)
	getDbFn    func(ctx context.Context, in *pb.GetDatabaseRequest, opts ...grpc.CallOption) (*pb.GetDatabaseResponse, error)
	listDbsFn  func(ctx context.Context, in *pb.ListDatabasesRequest, opts ...grpc.CallOption) (*pb.ListDatabasesResponse, error)
	getNodesFn func(ctx context.Context, in *pb.GetNodesRequest, opts ...grpc.CallOption) (*pb.GetNodesResponse, error)
}

func (m *mockMetadataClient) CreateDatabaseRecord(ctx context.Context, in *pb.CreateDatabaseRecordRequest, opts ...grpc.CallOption) (*pb.CreateDatabaseRecordResponse, error) {
	return m.createDbFn(ctx, in, opts...)
}
func (m *mockMetadataClient) UpdateDatabaseStatus(ctx context.Context, in *pb.UpdateDatabaseStatusRequest, opts ...grpc.CallOption) (*pb.UpdateDatabaseStatusResponse, error) {
	return m.updateDbFn(ctx, in, opts...)
}
func (m *mockMetadataClient) GetDatabase(ctx context.Context, in *pb.GetDatabaseRequest, opts ...grpc.CallOption) (*pb.GetDatabaseResponse, error) {
	return m.getDbFn(ctx, in, opts...)
}
func (m *mockMetadataClient) ListDatabases(ctx context.Context, in *pb.ListDatabasesRequest, opts ...grpc.CallOption) (*pb.ListDatabasesResponse, error) {
	return m.listDbsFn(ctx, in, opts...)
}
func (m *mockMetadataClient) GetNodes(ctx context.Context, in *pb.GetNodesRequest, opts ...grpc.CallOption) (*pb.GetNodesResponse, error) {
	return m.getNodesFn(ctx, in, opts...)
}

func TestValidationAndREST(t *testing.T) {
	mockMC := &mockMetadataClient{
		createDbFn: func(ctx context.Context, in *pb.CreateDatabaseRecordRequest, opts ...grpc.CallOption) (*pb.CreateDatabaseRecordResponse, error) {
			return &pb.CreateDatabaseRecordResponse{DatabaseId: "test-db-123"}, nil
		},
		getNodesFn: func(ctx context.Context, in *pb.GetNodesRequest, opts ...grpc.CallOption) (*pb.GetNodesResponse, error) {
			return &pb.GetNodesResponse{}, nil
		},
		updateDbFn: func(ctx context.Context, in *pb.UpdateDatabaseStatusRequest, opts ...grpc.CallOption) (*pb.UpdateDatabaseStatusResponse, error) {
			return &pb.UpdateDatabaseStatusResponse{Success: true}, nil
		},
	}
	mockSched := &mockSchedulerClient{
		schedFn: func(ctx context.Context, in *pb.ScheduleRequest, opts ...grpc.CallOption) (*pb.ScheduleResponse, error) {
			return &pb.ScheduleResponse{NodeId: "some-node"}, nil
		},
	}
	orch := NewOrchestrator(mockMC, mockSched)
	handlers := NewHandlers(mockMC, orch)

	mux := http.NewServeMux()
	handlers.RegisterRoutes(mux)

	// 1. Validate empty name
	t.Run("EmptyName", func(t *testing.T) {
		reqBody := `{"name":"","clusterId":"00000000-0000-0000-0000-000000000000"}`
		req := httptest.NewRequest("POST", "/v1/databases", strings.NewReader(reqBody))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d", w.Code)
		}
	})

	// 2. Validate invalid name pattern
	t.Run("InvalidNamePattern", func(t *testing.T) {
		reqBody := `{"name":"orders db; DROP TABLE databases;","clusterId":"00000000-0000-0000-0000-000000000000"}`
		req := httptest.NewRequest("POST", "/v1/databases", strings.NewReader(reqBody))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d", w.Code)
		}
	})

	// 3. Validate missing cluster ID
	t.Run("MissingClusterID", func(t *testing.T) {
		reqBody := `{"name":"orders-db","clusterId":""}`
		req := httptest.NewRequest("POST", "/v1/databases", strings.NewReader(reqBody))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d", w.Code)
		}
	})

	// 4. Success accepted 202
	t.Run("SuccessAccepted", func(t *testing.T) {
		reqBody := `{"name":"orders-db","clusterId":"00000000-0000-0000-0000-000000000000"}`
		req := httptest.NewRequest("POST", "/v1/databases", strings.NewReader(reqBody))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusAccepted {
			t.Errorf("expected 202 Accepted, got %d", w.Code)
		}
		var resp CreateDatabaseResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatal(err)
		}
		if resp.DatabaseID != "test-db-123" || resp.Status != "provisioning" {
			t.Errorf("unexpected response: %+v", resp)
		}
	})
}

// Mock Scheduler Client
type mockSchedulerClient struct {
	pb.SchedulerServiceClient
	schedFn func(ctx context.Context, in *pb.ScheduleRequest, opts ...grpc.CallOption) (*pb.ScheduleResponse, error)
}

func (m *mockSchedulerClient) Schedule(ctx context.Context, in *pb.ScheduleRequest, opts ...grpc.CallOption) (*pb.ScheduleResponse, error) {
	return m.schedFn(ctx, in, opts...)
}

func TestRetryLogic(t *testing.T) {
	var attempts atomic.Int32
	mockSched := &mockSchedulerClient{
		schedFn: func(ctx context.Context, in *pb.ScheduleRequest, opts ...grpc.CallOption) (*pb.ScheduleResponse, error) {
			attempts.Add(1)
			return &pb.ScheduleResponse{
				NodeId: "test-node",
				Score:  100.0,
			}, nil
		},
	}

	var updateAttempts atomic.Int32
	var finalStatus atomic.Value

	mockMC := &mockMetadataClient{
		getNodesFn: func(ctx context.Context, in *pb.GetNodesRequest, opts ...grpc.CallOption) (*pb.GetNodesResponse, error) {
			return &pb.GetNodesResponse{
				Nodes: []*pb.NodeInfo{
					{
						Id:       "test-node",
						Hostname: "localhost", // Dial localhost where no agent is listening to force connection failures
					},
				},
			}, nil
		},
		updateDbFn: func(ctx context.Context, in *pb.UpdateDatabaseStatusRequest, opts ...grpc.CallOption) (*pb.UpdateDatabaseStatusResponse, error) {
			updateAttempts.Add(1)
			finalStatus.Store(in.GetStatus())
			return &pb.UpdateDatabaseStatusResponse{Success: true}, nil
		},
	}

	orch := NewOrchestrator(mockMC, mockSched)

	// Since Node Agent is not listening on localhost:50053, it should fail dial and retry exactly 3 times
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	orch.ProvisionDatabase(ctx, "db-123", "orders", "cluster-123")

	// Verify scheduler called exactly 3 times
	if attempts.Load() != 3 {
		t.Errorf("expected 3 scheduled placement attempts, got %d", attempts.Load())
	}

	// Verify metadata status was marked as failed
	if finalStatus.Load() != "failed" {
		t.Errorf("expected final status to be 'failed', got %v", finalStatus.Load())
	}
}
