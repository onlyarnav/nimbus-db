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
