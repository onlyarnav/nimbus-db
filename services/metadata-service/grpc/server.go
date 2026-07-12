package grpc

import (
	"context"
	"errors"
	"log/slog"

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
	// Postgres will return a unique constraint error (23505) if the hostname already exists in that cluster,
	// and a foreign key constraint error (23503) if the cluster does not exist.
	var nodeID string
	query := "INSERT INTO nodes (cluster_id, hostname, status) VALUES ($1, $2, 'healthy') RETURNING id"
	err := s.db.QueryRow(ctx, query, clusterID, hostname).Scan(&nodeID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			slog.WarnContext(ctx, "database constraint violation on registration", "code", pgErr.Code, "message", pgErr.Message)
			// PostgreSQL state:
			// 23505 = unique_violation
			// 23503 = foreign_key_violation
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
