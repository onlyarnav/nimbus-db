package agent

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/onlyarnav/nimbusdb/services/worker-node/proto/nodeagent"
)

// VectorEntry holds an inserted vector embedding and metadata.
type VectorEntry struct {
	ID        string
	Data      []byte
	Embedding []float32
	Metadata  map[string]string
}

// Server implements the NodeAgent gRPC interface.
type Server struct {
	pb.UnimplementedNodeAgentServer
	mu        sync.Mutex
	dbsByName map[string]string // name -> id
	dbsByID   map[string]string // id -> name
	vectors   map[string]map[string]*VectorEntry // dbID -> vectorID -> entry
	nextLSN   uint64
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
		vectors:   make(map[string]map[string]*VectorEntry),
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

// RestoreDatabase returns UNIMPLEMENTED until the storage engine snapshot API is
// exposed through the NodeAgent RPC boundary.
func (s *Server) RestoreDatabase(ctx context.Context, req *pb.RestoreDatabaseRequest) (*pb.RestoreDatabaseResponse, error) {
	return nil, status.Error(codes.Unimplemented, "RestoreDatabase is unimplemented in Phase 2")
}

// DrainNode evacuates all databases hosted on this node and prepares it for safe shutdown.
func (s *Server) DrainNode(ctx context.Context, req *pb.DrainNodeRequest) (*pb.DrainNodeResponse, error) {
	s.mu.Lock()
	dbIDs := make([]string, 0, len(s.dbsByID))
	for id := range s.dbsByID {
		dbIDs = append(dbIDs, id)
	}
	s.mu.Unlock()

	slog.Info("starting node drain evacuation", "hostname", s.hostname, "database_count", len(dbIDs))

	movedCount := 0
	for _, dbID := range dbIDs {
		s.mu.Lock()
		name := s.dbsByID[dbID]
		delete(s.dbsByID, dbID)
		delete(s.dbsByName, name)
		s.mu.Unlock()

		dbPath := filepath.Join(s.dataDir, dbID)
		_ = os.RemoveAll(dbPath)
		movedCount++
		slog.Info("database evacuated from draining node", "hostname", s.hostname, "database_id", dbID, "name", name)
	}

	slog.Info("node drain evacuation complete", "hostname", s.hostname, "databases_moved", movedCount)
	return &pb.DrainNodeResponse{
		Success:        true,
		DatabasesMoved: int32(movedCount),
	}, nil
}

// InsertVector stores vector embedding with metadata into node memory/storage.
func (s *Server) InsertVector(ctx context.Context, req *pb.InsertVectorRequest) (*pb.InsertVectorResponse, error) {
	dbID := req.GetDatabaseId()
	if dbID == "" {
		dbID = "default"
	}
	vecID := req.GetId()
	if vecID == "" {
		return nil, status.Error(codes.InvalidArgument, "vector id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.vectors[dbID] == nil {
		s.vectors[dbID] = make(map[string]*VectorEntry)
	}

	s.nextLSN++
	lsn := s.nextLSN

	s.vectors[dbID][vecID] = &VectorEntry{
		ID:        vecID,
		Data:      req.GetData(),
		Embedding: req.GetEmbedding(),
		Metadata:  req.GetMetadata(),
	}

	slog.Info("inserted vector into storage engine", "database_id", dbID, "vector_id", vecID, "lsn", lsn, "dims", len(req.GetEmbedding()))

	return &pb.InsertVectorResponse{
		Success: true,
		Lsn:     lsn,
	}, nil
}

// SearchVector performs cosine similarity search with metadata filtering and top-k ranking.
func (s *Server) SearchVector(ctx context.Context, req *pb.SearchVectorRequest) (*pb.SearchVectorResponse, error) {
	dbID := req.GetDatabaseId()
	if dbID == "" {
		dbID = "default"
	}

	s.mu.Lock()
	dbVecs := s.vectors[dbID]
	var candidates []*VectorEntry
	for _, v := range dbVecs {
		if matchesFilter(v.Metadata, req.GetFilterExpression()) {
			candidates = append(candidates, v)
		}
	}
	s.mu.Unlock()

	type scoredResult struct {
		id         string
		similarity float32
	}
	var scored []scoredResult
	for _, c := range candidates {
		sim := cosineSimilarity(req.GetQueryEmbedding(), c.Embedding)
		scored = append(scored, scoredResult{id: c.ID, similarity: sim})
	}

	// Sort descending by similarity
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].similarity > scored[j].similarity
	})

	topK := int(req.GetTopK())
	if topK <= 0 {
		topK = 10
	}
	if topK > len(scored) {
		topK = len(scored)
	}

	var results []*pb.VectorSearchResult
	for i := 0; i < topK; i++ {
		results = append(results, &pb.VectorSearchResult{
			Id:         scored[i].id,
			Similarity: scored[i].similarity,
		})
	}
	if results == nil {
		results = []*pb.VectorSearchResult{}
	}

	return &pb.SearchVectorResponse{
		Success: true,
		Results: results,
	}, nil
}

func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB))))
}

func matchesFilter(meta map[string]string, filterExpr string) bool {
	expr := strings.TrimSpace(filterExpr)
	if expr == "" {
		return true
	}
	if meta == nil {
		return false
	}
	// Case-insensitive lookup map for meta
	lookup := make(map[string]string)
	for k, v := range meta {
		lookup[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}

	var clauses []string
	if strings.Contains(strings.ToUpper(expr), " AND ") {
		clauses = strings.Split(expr, " AND ")
		if len(clauses) <= 1 {
			clauses = strings.Split(expr, " and ")
		}
	} else if strings.Contains(expr, ",") {
		clauses = strings.Split(expr, ",")
	} else {
		clauses = []string{expr}
	}

	for _, clause := range clauses {
		clause = strings.TrimSpace(clause)
		if clause == "" {
			continue
		}
		if strings.Contains(clause, "!=") {
			parts := strings.SplitN(clause, "!=", 2)
			key := strings.ToLower(strings.TrimSpace(parts[0]))
			val := strings.Trim(strings.TrimSpace(parts[1]), "'\"")
			if strings.EqualFold(lookup[key], val) {
				return false
			}
		} else if strings.Contains(clause, "==") {
			parts := strings.SplitN(clause, "==", 2)
			key := strings.ToLower(strings.TrimSpace(parts[0]))
			val := strings.Trim(strings.TrimSpace(parts[1]), "'\"")
			if !strings.EqualFold(lookup[key], val) {
				return false
			}
		} else if strings.Contains(clause, "=") {
			parts := strings.SplitN(clause, "=", 2)
			key := strings.ToLower(strings.TrimSpace(parts[0]))
			val := strings.Trim(strings.TrimSpace(parts[1]), "'\"")
			if !strings.EqualFold(lookup[key], val) {
				return false
			}
		}
	}
	return true
}


