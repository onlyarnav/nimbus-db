package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/onlyarnav/nimbusdb/services/gateway/router"
	"github.com/onlyarnav/nimbusdb/services/metadata-service/region"
	pb "github.com/onlyarnav/nimbusdb/services/metadata-service/proto"
)

type CreateDatabaseRequest struct {
	Name            string `json:"name"`
	ClusterID       string `json:"clusterId"`
	PreferredRegion string `json:"preferredRegion,omitempty"`
}

type CreateDatabaseResponse struct {
	DatabaseID       string `json:"databaseId"`
	Status           string `json:"status"`
	PreferredRegion  string `json:"preferredRegion"`
	ServedRegion     string `json:"servedRegion"`
	FallbackRerouted bool   `json:"fallbackRerouted"`
	Reason           string `json:"reason,omitempty"`
}

type RegionHealthResponse struct {
	Regions       []region.RegionHealthInfo `json:"regions"`
	LatencyMatrix map[string]map[string]int  `json:"latencyMatrix"`
}

type GatewayHandlers struct {
	metadataClient pb.MetadataServiceClient
}

func NewGatewayHandlers(mc pb.MetadataServiceClient) *GatewayHandlers {
	return &GatewayHandlers{metadataClient: mc}
}

func (g *GatewayHandlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/databases", g.handleCreateDatabase)
	mux.HandleFunc("GET /v1/databases/{id}", g.handleGetDatabase)
	mux.HandleFunc("GET /v1/databases", g.handleListDatabases)
	mux.HandleFunc("DELETE /v1/databases/{id}", g.handleDeleteDatabase)
	mux.HandleFunc("GET /v1/regions", g.handleListRegions)
	mux.HandleFunc("GET /health", g.handleHealth)
}

func (g *GatewayHandlers) handleCreateDatabase(w http.ResponseWriter, r *http.Request) {
	var req CreateDatabaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "database name is required", http.StatusBadRequest)
		return
	}
	if req.ClusterID == "" {
		req.ClusterID = "default-cluster"
	}

	ctx := r.Context()

	// 1. Fetch current node health state from Metadata Service
	nodesRes, err := g.metadataClient.GetNodes(ctx, &pb.GetNodesRequest{})
	var nodeStates []region.NodeState
	if err == nil {
		for _, n := range nodesRes.GetNodes() {
			nodeStates = append(nodeStates, region.NodeState{
				ID:     n.GetId(),
				Region: n.GetClusterId(), // or hostname prefix
				Status: n.GetStatus(),
			})
		}
	}

	// Calculate region health map
	regionHealthMap := make(map[string]region.RegionStatus)
	for _, reg := range region.SupportedRegions {
		rInfo := region.RollupRegionHealth(reg, nodeStates)
		regionHealthMap[reg] = rInfo.Status
	}

	// 2. Select region via Gateway Router
	routeRes, err := router.SelectRegion(req.PreferredRegion, regionHealthMap)
	if err != nil {
		slog.ErrorContext(ctx, "failed to route database creation request", "error", err)
		http.Error(w, "routing failed: "+err.Error(), http.StatusServiceUnavailable)
		return
	}

	// 3. Register database record in Metadata Service
	metaRes, err := g.metadataClient.CreateDatabaseRecord(ctx, &pb.CreateDatabaseRecordRequest{
		Name:      req.Name,
		ClusterId: req.ClusterID,
		Status:    "provisioning",
		Attempts:  1,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to register database in metadata service", "error", err)
		http.Error(w, "internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	dbID := metaRes.GetDatabaseId()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(CreateDatabaseResponse{
		DatabaseID:       dbID,
		Status:           "provisioning",
		PreferredRegion:  routeRes.PreferredRegion,
		ServedRegion:     routeRes.ServedRegion,
		FallbackRerouted: routeRes.FallbackRerouted,
		Reason:           routeRes.Reason,
	})
}

func (g *GatewayHandlers) handleGetDatabase(w http.ResponseWriter, r *http.Request) {
	dbID := r.PathValue("id")
	if dbID == "" {
		http.Error(w, "database id is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	metaRes, err := g.metadataClient.GetDatabase(ctx, &pb.GetDatabaseRequest{DatabaseId: dbID})
	if err != nil {
		http.Error(w, "database not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(metaRes.GetDatabase())
}

func (g *GatewayHandlers) handleListDatabases(w http.ResponseWriter, r *http.Request) {
	clusterID := r.URL.Query().Get("clusterId")
	ctx := r.Context()

	metaRes, err := g.metadataClient.ListDatabases(ctx, &pb.ListDatabasesRequest{ClusterId: clusterID})
	if err != nil {
		http.Error(w, "internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(metaRes.GetDatabases())
}

func (g *GatewayHandlers) handleDeleteDatabase(w http.ResponseWriter, r *http.Request) {
	dbID := r.PathValue("id")
	if dbID == "" {
		http.Error(w, "database id is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	_, err := g.metadataClient.DeleteDatabaseRecord(ctx, &pb.DeleteDatabaseRecordRequest{DatabaseId: dbID})
	if err != nil {
		http.Error(w, "failed to delete database: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (g *GatewayHandlers) handleListRegions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	nodesRes, err := g.metadataClient.GetNodes(ctx, &pb.GetNodesRequest{})

	var nodeStates []region.NodeState
	if err == nil {
		for _, n := range nodesRes.GetNodes() {
			reg := n.GetClusterId()
			if reg == "" || reg == "default-cluster" {
				reg = region.RegionIndia
			}
			nodeStates = append(nodeStates, region.NodeState{
				ID:     n.GetId(),
				Region: reg,
				Status: n.GetStatus(),
			})
		}
	}

	var rList []region.RegionHealthInfo
	for _, regName := range region.SupportedRegions {
		rInfo := region.RollupRegionHealth(regName, nodeStates)
		rList = append(rList, rInfo)
	}

	resp := RegionHealthResponse{
		Regions:       rList,
		LatencyMatrix: region.LatencyMatrix,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (g *GatewayHandlers) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "UP", "service": "gateway"})
}
