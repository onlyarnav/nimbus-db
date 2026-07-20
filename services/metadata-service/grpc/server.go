package grpc

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/onlyarnav/nimbusdb/services/metadata-service/proto"
)

// Server implements the generated MetadataServiceServer interface.
type Server struct {
	pb.UnimplementedMetadataServiceServer
	db *pgxpool.Pool
}

// NewServer creates a new instance of our gRPC server.
func NewServer(db *pgxpool.Pool) *Server {
	return &Server{db: db}
}

// RegisterNode registers a new worker node in a specific cluster, validating constraints.
func (s *Server) RegisterNode(ctx context.Context, req *pb.RegisterNodeRequest) (*pb.RegisterNodeResponse, error) {
	clusterID := req.GetClusterId()
	hostname := req.GetHostname()

	// 1. Validate parameters are not empty
	if clusterID == "" {
		return nil, status.Error(codes.InvalidArgument, "cluster_id is required")
	}
	if hostname == "" {
		return nil, status.Error(codes.InvalidArgument, "hostname is required")
	}

	slog.InfoContext(ctx, "received register node request", "cluster_id", clusterID, "hostname", hostname)

	// 2. Insert into the nodes table.
	var nodeID string
	query := "INSERT INTO nodes (cluster_id, hostname, status) VALUES ($1, $2, 'healthy') RETURNING id"
	err := s.db.QueryRow(ctx, query, clusterID, hostname).Scan(&nodeID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			slog.WarnContext(ctx, "database constraint violation on registration", "code", pgErr.Code, "message", pgErr.Message)
			if pgErr.Code == "23505" {
				return nil, status.Errorf(codes.AlreadyExists, "node with hostname %q already exists in cluster %q", hostname, clusterID)
			}
			if pgErr.Code == "23503" {
				return nil, status.Errorf(codes.NotFound, "cluster %q does not exist", clusterID)
			}
		}
		slog.ErrorContext(ctx, "failed to register node in database", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to register node: %v", err)
	}

	slog.InfoContext(ctx, "node registered successfully", "node_id", nodeID, "hostname", hostname)

	return &pb.RegisterNodeResponse{
		NodeId:                   nodeID,
		HeartbeatIntervalSeconds: 5,
	}, nil
}

// SendHeartbeat processes incoming stats from a worker node and updates its health.
func (s *Server) SendHeartbeat(ctx context.Context, req *pb.SendHeartbeatRequest) (*pb.SendHeartbeatResponse, error) {
	nodeID := req.GetNodeId()
	if nodeID == "" {
		return nil, status.Error(codes.InvalidArgument, "node_id is required")
	}

	slog.InfoContext(ctx, "received heartbeat request", "node_id", nodeID, "cpu", req.GetCpuPct(), "memory", req.GetMemoryPct(), "disk", req.GetDiskPct(), "healthy", req.GetHealthy())

	// Start a transaction to guarantee atomic insert of heartbeat and update of nodes table
	tx, err := s.db.Begin(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to start transaction", "error", err)
		return nil, status.Errorf(codes.Internal, "transaction failed: %v", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	// 1. Insert append-only heartbeat record
	insertQuery := `INSERT INTO heartbeats (node_id, cpu_pct, memory_pct, disk_pct, healthy, received_at)
	                VALUES ($1, $2, $3, $4, $5, now())`
	_, err = tx.Exec(ctx, insertQuery, nodeID, req.GetCpuPct(), req.GetMemoryPct(), req.GetDiskPct(), req.GetHealthy())
	if err != nil {
		slog.ErrorContext(ctx, "failed to insert heartbeat", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to save heartbeat: %v", err)
	}

	// 2. Denormalize current state into nodes table
	// If the worker reports itself unhealthy, set status = 'unhealthy'
	var newStatus string
	if !req.GetHealthy() {
		newStatus = "unhealthy"
	}

	var updateQuery string
	var args []interface{}
	if newStatus != "" {
		updateQuery = `UPDATE nodes
		               SET cpu_pct = $1, memory_pct = $2, disk_pct = $3, last_heartbeat = now(), status = $4
		               WHERE id = $5`
		args = []interface{}{req.GetCpuPct(), req.GetMemoryPct(), req.GetDiskPct(), newStatus, nodeID}
	} else {
		// If the worker was classified as dead/unhealthy/unknown previously, make it healthy again upon heartbeat
		updateQuery = `UPDATE nodes
		               SET cpu_pct = $1, memory_pct = $2, disk_pct = $3, last_heartbeat = now(),
		                   status = CASE WHEN status IN ('dead', 'unhealthy', 'unknown') THEN 'healthy' ELSE status END
		               WHERE id = $4`
		args = []interface{}{req.GetCpuPct(), req.GetMemoryPct(), req.GetDiskPct(), nodeID}
	}

	cmd, err := tx.Exec(ctx, updateQuery, args...)
	if err != nil {
		slog.ErrorContext(ctx, "failed to update node status", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to update node: %v", err)
	}
	if cmd.RowsAffected() == 0 {
		return nil, status.Errorf(codes.NotFound, "node %q not found", nodeID)
	}

	if err := tx.Commit(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to commit transaction", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to commit: %v", err)
	}

	return &pb.SendHeartbeatResponse{Success: true}, nil
}

// GetNodes fetches the current state of all registered nodes in the cluster.
func (s *Server) GetNodes(ctx context.Context, req *pb.GetNodesRequest) (*pb.GetNodesResponse, error) {
	clusterID := req.GetClusterId()

	var rows pgx.Rows
	var err error

	if clusterID != "" {
		query := `SELECT id, cluster_id, hostname, status, COALESCE(cpu_pct, 0), COALESCE(memory_pct, 0), COALESCE(disk_pct, 0), last_heartbeat, registered_at
		         FROM nodes WHERE cluster_id = $1`
		rows, err = s.db.Query(ctx, query, clusterID)
	} else {
		query := `SELECT id, cluster_id, hostname, status, COALESCE(cpu_pct, 0), COALESCE(memory_pct, 0), COALESCE(disk_pct, 0), last_heartbeat, registered_at
		         FROM nodes`
		rows, err = s.db.Query(ctx, query)
	}

	if err != nil {
		slog.ErrorContext(ctx, "failed to query nodes", "error", err)
		return nil, status.Errorf(codes.Internal, "query failed: %v", err)
	}
	defer rows.Close()

	var nodes []*pb.NodeInfo
	for rows.Next() {
		var n pb.NodeInfo
		var cpu, mem, disk float32
		var lastHB *time.Time
		var regAt time.Time
		err := rows.Scan(&n.Id, &n.ClusterId, &n.Hostname, &n.Status, &cpu, &mem, &disk, &lastHB, &regAt)
		if err != nil {
			slog.ErrorContext(ctx, "failed to scan node row", "error", err)
			return nil, status.Errorf(codes.Internal, "scan failed: %v", err)
		}
		n.CpuPct = cpu
		n.MemoryPct = mem
		n.DiskPct = disk
		n.RegisteredAt = regAt.Format(time.RFC3339)
		if lastHB != nil {
			n.LastHeartbeat = lastHB.Format(time.RFC3339)
		}
		nodes = append(nodes, &n)
	}

	return &pb.GetNodesResponse{Nodes: nodes}, nil
}

// CreateDatabaseRecord inserts a new database and leader replica into the registry.
func (s *Server) CreateDatabaseRecord(ctx context.Context, req *pb.CreateDatabaseRecordRequest) (*pb.CreateDatabaseRecordResponse, error) {
	name := req.GetName()
	clusterID := req.GetClusterId()
	nodeID := req.GetNodeId()
	statusStr := req.GetStatus()
	attempts := req.GetAttempts()

	if name == "" || clusterID == "" || statusStr == "" {
		return nil, status.Error(codes.InvalidArgument, "name, cluster_id, and status are required")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to start transaction", "error", err)
		return nil, status.Errorf(codes.Internal, "transaction failed: %v", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var dbID string
	var insertQuery string
	var args []interface{}

	if nodeID != "" {
		insertQuery = `INSERT INTO databases (name, cluster_id, node_id, status, attempts, created_at, updated_at)
		               VALUES ($1, $2, $3, $4, $5, now(), now()) RETURNING id`
		args = []interface{}{name, clusterID, nodeID, statusStr, attempts}
	} else {
		insertQuery = `INSERT INTO databases (name, cluster_id, status, attempts, created_at, updated_at)
		               VALUES ($1, $2, $3, $4, now(), now()) RETURNING id`
		args = []interface{}{name, clusterID, statusStr, attempts}
	}

	err = tx.QueryRow(ctx, insertQuery, args...).Scan(&dbID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to insert database record", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to insert database: %v", err)
	}

	// If node ID is provided, insert a leader replica
	if nodeID != "" {
		repQuery := `INSERT INTO replicas (database_id, node_id, role) VALUES ($1, $2, 'leader')`
		_, err = tx.Exec(ctx, repQuery, dbID, nodeID)
		if err != nil {
			slog.ErrorContext(ctx, "failed to insert leader replica", "error", err)
			return nil, status.Errorf(codes.Internal, "failed to insert replica: %v", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to commit transaction", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to commit: %v", err)
	}

	return &pb.CreateDatabaseRecordResponse{DatabaseId: dbID}, nil
}

// UpdateDatabaseStatus updates database metadata (status, node_id, endpoint, attempts).
func (s *Server) UpdateDatabaseStatus(ctx context.Context, req *pb.UpdateDatabaseStatusRequest) (*pb.UpdateDatabaseStatusResponse, error) {
	dbID := req.GetDatabaseId()
	statusStr := req.GetStatus()
	nodeID := req.GetNodeId()
	endpoint := req.GetEndpoint()
	attempts := req.GetAttempts()

	if dbID == "" || statusStr == "" {
		return nil, status.Error(codes.InvalidArgument, "database_id and status are required")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to start transaction", "error", err)
		return nil, status.Errorf(codes.Internal, "transaction failed: %v", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	// Update database details. If nodeID is empty, we don't update node_id.
	var updateQuery string
	var args []interface{}
	if nodeID != "" {
		updateQuery = `UPDATE databases
		               SET status = $1, node_id = $2, endpoint = $3, attempts = $4, updated_at = now()
		               WHERE id = $5`
		args = []interface{}{statusStr, nodeID, endpoint, attempts, dbID}
	} else {
		updateQuery = `UPDATE databases
		               SET status = $1, endpoint = $2, attempts = $3, updated_at = now()
		               WHERE id = $4`
		args = []interface{}{statusStr, endpoint, attempts, dbID}
	}

	cmd, err := tx.Exec(ctx, updateQuery, args...)
	if err != nil {
		slog.ErrorContext(ctx, "failed to update database status", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to update database status: %v", err)
	}
	if cmd.RowsAffected() == 0 {
		return nil, status.Errorf(codes.NotFound, "database %q not found", dbID)
	}

	// Update leader replica if nodeID changed/is updated
	if nodeID != "" {
		// Delete any existing replicas for this database and insert the new leader
		_, err = tx.Exec(ctx, `DELETE FROM replicas WHERE database_id = $1`, dbID)
		if err != nil {
			slog.ErrorContext(ctx, "failed to clear replicas", "error", err)
			return nil, status.Errorf(codes.Internal, "failed to clear replicas: %v", err)
		}

		repQuery := `INSERT INTO replicas (database_id, node_id, role) VALUES ($1, $2, 'leader')`
		_, err = tx.Exec(ctx, repQuery, dbID, nodeID)
		if err != nil {
			slog.ErrorContext(ctx, "failed to update leader replica", "error", err)
			return nil, status.Errorf(codes.Internal, "failed to update replica: %v", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to commit transaction", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to commit: %v", err)
	}

	return &pb.UpdateDatabaseStatusResponse{Success: true}, nil
}

// GetDatabase retrieves info for a specific database.
func (s *Server) GetDatabase(ctx context.Context, req *pb.GetDatabaseRequest) (*pb.GetDatabaseResponse, error) {
	dbID := req.GetDatabaseId()
	if dbID == "" {
		return nil, status.Error(codes.InvalidArgument, "database_id is required")
	}

	query := `SELECT id, name, cluster_id, node_id, status, endpoint, attempts, created_at, updated_at
	          FROM databases WHERE id = $1`
	var dbInfo pb.DatabaseInfo
	var nodeID *string
	var endpoint *string
	var regAt, upAt time.Time

	err := s.db.QueryRow(ctx, query, dbID).Scan(
		&dbInfo.Id, &dbInfo.Name, &dbInfo.ClusterId, &nodeID, &dbInfo.Status, &endpoint, &dbInfo.Attempts, &regAt, &upAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, status.Errorf(codes.NotFound, "database %q not found", dbID)
		}
		slog.ErrorContext(ctx, "failed to fetch database record", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to fetch database: %v", err)
	}

	if nodeID != nil {
		dbInfo.NodeId = *nodeID
	}
	if endpoint != nil {
		dbInfo.Endpoint = *endpoint
	}
	dbInfo.CreatedAt = regAt.Format(time.RFC3339)
	dbInfo.UpdatedAt = upAt.Format(time.RFC3339)

	return &pb.GetDatabaseResponse{Database: &dbInfo}, nil
}

// ListDatabases lists all registered databases, optionally filtering by cluster_id.
func (s *Server) ListDatabases(ctx context.Context, req *pb.ListDatabasesRequest) (*pb.ListDatabasesResponse, error) {
	clusterID := req.GetClusterId()

	var rows pgx.Rows
	var err error

	if clusterID != "" {
		query := `SELECT id, name, cluster_id, node_id, status, endpoint, attempts, created_at, updated_at
		          FROM databases WHERE cluster_id = $1`
		rows, err = s.db.Query(ctx, query, clusterID)
	} else {
		query := `SELECT id, name, cluster_id, node_id, status, endpoint, attempts, created_at, updated_at
		          FROM databases`
		rows, err = s.db.Query(ctx, query)
	}

	if err != nil {
		slog.ErrorContext(ctx, "failed to query databases", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to list databases: %v", err)
	}
	defer rows.Close()

	var list []*pb.DatabaseInfo
	for rows.Next() {
		var dbInfo pb.DatabaseInfo
		var nodeID *string
		var endpoint *string
		var regAt, upAt time.Time

		err := rows.Scan(
			&dbInfo.Id, &dbInfo.Name, &dbInfo.ClusterId, &nodeID, &dbInfo.Status, &endpoint, &dbInfo.Attempts, &regAt, &upAt,
		)
		if err != nil {
			slog.ErrorContext(ctx, "failed to scan database row", "error", err)
			return nil, status.Errorf(codes.Internal, "failed to scan databases list: %v", err)
		}

		if nodeID != nil {
			dbInfo.NodeId = *nodeID
		}
		if endpoint != nil {
			dbInfo.Endpoint = *endpoint
		}
		dbInfo.CreatedAt = regAt.Format(time.RFC3339)
		dbInfo.UpdatedAt = upAt.Format(time.RFC3339)

		list = append(list, &dbInfo)
	}

	return &pb.ListDatabasesResponse{Databases: list}, nil
}

// DeleteDatabaseRecord removes database and associated replica metadata.
func (s *Server) DeleteDatabaseRecord(ctx context.Context, req *pb.DeleteDatabaseRecordRequest) (*pb.DeleteDatabaseRecordResponse, error) {
	dbID := req.GetDatabaseId()
	if dbID == "" {
		return nil, status.Error(codes.InvalidArgument, "database_id is required")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to start transaction", "error", err)
		return nil, status.Errorf(codes.Internal, "transaction failed: %v", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	// Delete from replicas first
	_, err = tx.Exec(ctx, `DELETE FROM replicas WHERE database_id = $1`, dbID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to delete replicas", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to delete database replicas: %v", err)
	}

	cmd, err := tx.Exec(ctx, `DELETE FROM databases WHERE id = $1`, dbID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to delete database record", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to delete database: %v", err)
	}

	if cmd.RowsAffected() == 0 {
		return nil, status.Errorf(codes.NotFound, "database %q not found", dbID)
	}

	if err := tx.Commit(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to commit transaction", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to commit: %v", err)
	}

	return &pb.DeleteDatabaseRecordResponse{Success: true}, nil
}

