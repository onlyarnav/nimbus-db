package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/onlyarnav/nimbusdb/services/worker-node/proto/nodeagent"
)

// Server implements the NodeAgent gRPC interface.
type Server struct {
	pb.UnimplementedNodeAgentServer
	mu        sync.Mutex
	dbsByName map[string]string // name -> id
	dbsByID   map[string]string // id -> name
	dataDir   string
	hostname  string

	// Failure injection counters
	FailAttempts int32
	HangAttempts int32
}

// NewServer creates a new instance of the NodeAgent server.
func NewServer(dataDir, hostname string) *Server {
	return &Server{
		dbsByName: make(map[string]string),
		dbsByID:   make(map[string]string),
		dataDir:   dataDir,
		hostname:  hostname,
	}
}

// CreateDatabase handles directory allocation and name uniqueness checks.
func (s *Server) CreateDatabase(ctx context.Context, req *pb.CreateDatabaseRequest) (*pb.CreateDatabaseResponse, error) {
	name := req.GetName()
	dbID := req.GetDatabaseId()

	if name == "" || dbID == "" {
		return nil, status.Error(codes.InvalidArgument, "name and database_id are required")
	}

	// 1. Check failure injection triggers
	if atomic.LoadInt32(&s.FailAttempts) > 0 {
		atomic.AddInt32(&s.FailAttempts, -1)
		slog.Warn("simulated failure injected for CreateDatabase", "name", name, "id", dbID)
		return &pb.CreateDatabaseResponse{
			Success: false,
			Error:   "simulated agent creation failure",
		}, nil
	}

	if atomic.LoadInt32(&s.HangAttempts) > 0 {
		atomic.AddInt32(&s.HangAttempts, -1)
		slog.Warn("simulated hang injected for CreateDatabase, sleeping 15s...", "name", name, "id", dbID)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(15 * time.Second):
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 2. Reject duplicate database names/IDs on the same node
	if existingID, ok := s.dbsByName[name]; ok {
		if existingID != dbID {
			return nil, status.Errorf(codes.AlreadyExists, "database with name %q already exists on this node", name)
		}
	}
	if existingName, ok := s.dbsByID[dbID]; ok {
		if existingName != name {
			return nil, status.Errorf(codes.AlreadyExists, "database with ID %q already exists on this node", dbID)
		}
	}

	// 3. Allocate directory namespace
	dbPath := filepath.Join(s.dataDir, dbID)
	if err := os.MkdirAll(dbPath, 0755); err != nil {
		slog.Error("failed to create database namespace directory", "error", err, "path", dbPath)
		return nil, status.Errorf(codes.Internal, "failed to allocate directory: %v", err)
	}

	// 4. Register locally
	s.dbsByName[name] = dbID
	s.dbsByID[dbID] = name

	slog.Info("database created locally on node", "hostname", s.hostname, "name", name, "id", dbID, "path", dbPath)

	endpoint := fmt.Sprintf("%s/db/%s", s.hostname, dbID)
	return &pb.CreateDatabaseResponse{
		Success:  true,
		Endpoint: endpoint,
	}, nil
}

// DeleteDatabase deletes local database directories and state maps.
func (s *Server) DeleteDatabase(ctx context.Context, req *pb.DeleteDatabaseRequest) (*pb.DeleteDatabaseResponse, error) {
	dbID := req.GetDatabaseId()
	if dbID == "" {
		return nil, status.Error(codes.InvalidArgument, "database_id is required")
	}

	s.mu.Lock()
	name, ok := s.dbsByID[dbID]
	if !ok {
		s.mu.Unlock()
		return nil, status.Errorf(codes.NotFound, "database with ID %q not found", dbID)
	}
	delete(s.dbsByID, dbID)
	delete(s.dbsByName, name)
	s.mu.Unlock()

	// Delete directory namespace
	dbPath := filepath.Join(s.dataDir, dbID)
	_ = os.RemoveAll(dbPath)

	slog.Info("database deleted locally from node", "hostname", s.hostname, "id", dbID)
	return &pb.DeleteDatabaseResponse{Success: true}, nil
}

// BackupDatabase returns UNIMPLEMENTED gRPC code in Phase 2.
func (s *Server) BackupDatabase(ctx context.Context, req *pb.BackupDatabaseRequest) (*pb.BackupDatabaseResponse, error) {
	return nil, status.Error(codes.Unimplemented, "BackupDatabase is unimplemented in Phase 2")
}

// RestoreDatabase returns UNIMPLEMENTED gRPC code in Phase 2.
func (s *Server) RestoreDatabase(ctx context.Context, req *pb.RestoreDatabaseRequest) (*pb.RestoreDatabaseResponse, error) {
	return nil, status.Error(codes.Unimplemented, "RestoreDatabase is unimplemented in Phase 2")
}
