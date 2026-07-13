package grpc

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/onlyarnav/nimbusdb/services/scheduler/placement"
	pb "github.com/onlyarnav/nimbusdb/services/scheduler/proto"
)

// Server implements the generated SchedulerServiceServer interface.
type Server struct {
	pb.UnimplementedSchedulerServiceServer
	metaClient pb.MetadataServiceClient
}

// NewServer initializes a new Server instance.
func NewServer(metaClient pb.MetadataServiceClient) *Server {
	return &Server{metaClient: metaClient}
}

// Schedule coordinates metadata retrieval and node placement decision.
func (s *Server) Schedule(ctx context.Context, req *pb.ScheduleRequest) (*pb.ScheduleResponse, error) {
	clusterID := req.GetClusterId()
	if clusterID == "" {
		return nil, status.Error(codes.InvalidArgument, "cluster_id is required")
	}

	slog.InfoContext(ctx, "received scheduling request", "cluster_id", clusterID)

	// 1. Fetch current node topology from Metadata Service
	res, err := s.metaClient.GetNodes(ctx, &pb.GetNodesRequest{ClusterId: clusterID})
	if err != nil {
		slog.ErrorContext(ctx, "failed to retrieve nodes from metadata service", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to get cluster nodes: %v", err)
	}

	// 2. Compute placement score and filters
	bestNode, err := placement.ScheduleNode(res.GetNodes())
	if err != nil {
		if err == placement.ErrNoNodesAvailable {
			return nil, status.Error(codes.ResourceExhausted, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "placement calculation failed: %v", err)
	}

	slog.InfoContext(ctx, "placement decision determined", "node_id", bestNode.NodeID, "score", bestNode.Score)

	return &pb.ScheduleResponse{
		NodeId:         bestNode.NodeID,
		Score:          bestNode.Score,
		ScoreBreakdown: bestNode.ScoreBreakdown,
	}, nil
}
