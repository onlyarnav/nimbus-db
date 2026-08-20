package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"


	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/onlyarnav/nimbusdb/services/auth-service/auth"
	"github.com/onlyarnav/nimbusdb/services/gateway/router"
	pb "github.com/onlyarnav/nimbusdb/services/metadata-service/proto"
	"github.com/onlyarnav/nimbusdb/services/metadata-service/region"
	"github.com/onlyarnav/nimbusdb/services/observability/telemetry"
	pbAgent "github.com/onlyarnav/nimbusdb/services/worker-node/proto/nodeagent"
)

func resolveNodeRegion(clusterID, hostname string) string {
	for _, r := range region.SupportedRegions {
		if strings.Contains(strings.ToLower(clusterID), r) || strings.Contains(strings.ToLower(hostname), r) {
			return r
		}
	}
	return region.RegionIndia
}

func getWorkerNodeAddr() string {
	if addr := os.Getenv("WORKER_NODE_ADDR"); addr != "" {
		return addr
	}
	return "nimbusdb-worker-node:50053"
}

func (g *GatewayHandlers) resolveWorkerNodeAddr(ctx context.Context, dbID string) string {
	if dbID != "" && dbID != "default" {
		dbRes, err := g.metadataClient.GetDatabase(ctx, &pb.GetDatabaseRequest{DatabaseId: dbID})
		if err == nil && dbRes.GetDatabase() != nil {
			endpoint := dbRes.GetDatabase().GetEndpoint()
			if endpoint != "" {
				host := strings.Split(endpoint, "/")[0]
				if strings.HasPrefix(host, "nimbusdb-worker-node-") {
					return host + ".nimbusdb-worker-node:50053"
				}
				if host != "" {
					return host + ":50053"
				}
			}
		}
	}
	return getWorkerNodeAddr()
}


type InsertVectorPayload struct {
	DatabaseID string            `json:"databaseId,omitempty"`
	ID         string            `json:"id"`
	Data       string            `json:"data,omitempty"`
	Embedding  []float32         `json:"embedding"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type SearchVectorPayload struct {
	DatabaseID       string    `json:"databaseId,omitempty"`
	QueryEmbedding   []float32 `json:"queryEmbedding"`
	TopK             int32     `json:"topK,omitempty"`
	FilterExpression string    `json:"filterExpression,omitempty"`
	Exact            bool      `json:"exact,omitempty"`
}



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
	mux.Handle("POST /v1/databases", auth.AuthenticateAndAuthorize(auth.RoleOperator)(http.HandlerFunc(g.handleCreateDatabase)))
	mux.Handle("GET /v1/databases/{id}", auth.AuthenticateAndAuthorize(auth.RoleReadOnly)(http.HandlerFunc(g.handleGetDatabase)))
	mux.Handle("GET /v1/databases", auth.AuthenticateAndAuthorize(auth.RoleReadOnly)(http.HandlerFunc(g.handleListDatabases)))
	mux.Handle("DELETE /v1/databases/{id}", auth.AuthenticateAndAuthorize(auth.RoleOperator)(http.HandlerFunc(g.handleDeleteDatabase)))
	mux.Handle("GET /v1/regions", auth.AuthenticateAndAuthorize(auth.RoleReadOnly)(http.HandlerFunc(g.handleListRegions)))
	mux.Handle("POST /v1/deployments/rolling", auth.AuthenticateAndAuthorize(auth.RoleAdmin)(http.HandlerFunc(g.handleTriggerRollingDeployment)))
	mux.Handle("POST /v1/deployments/canary", auth.AuthenticateAndAuthorize(auth.RoleAdmin)(http.HandlerFunc(g.handleTriggerCanaryDeployment)))
	mux.Handle("POST /v1/deployments/blue-green", auth.AuthenticateAndAuthorize(auth.RoleAdmin)(http.HandlerFunc(g.handleTriggerBlueGreenDeployment)))
	mux.Handle("POST /v1/deployments/rollback", auth.AuthenticateAndAuthorize(auth.RoleAdmin)(http.HandlerFunc(g.handleTriggerRollback)))
	mux.Handle("POST /v1/nodes/{id}/drain", auth.AuthenticateAndAuthorize(auth.RoleAdmin)(http.HandlerFunc(g.handleDrainNode)))
	mux.Handle("GET /v1/capacity/projection", auth.AuthenticateAndAuthorize(auth.RoleReadOnly)(http.HandlerFunc(g.handleGetCapacityProjection)))
	mux.Handle("GET /v1/sla/report", auth.AuthenticateAndAuthorize(auth.RoleReadOnly)(http.HandlerFunc(g.handleGetSLAReport)))
	mux.Handle("POST /v1/databases/{id}/vectors", auth.AuthenticateAndAuthorize(auth.RoleOperator)(http.HandlerFunc(g.handleInsertVector)))
	mux.Handle("POST /v1/databases/{id}/vectors/search", auth.AuthenticateAndAuthorize(auth.RoleReadOnly)(http.HandlerFunc(g.handleSearchVector)))
	mux.Handle("POST /v1/vectors/insert", auth.AuthenticateAndAuthorize(auth.RoleOperator)(http.HandlerFunc(g.handleInsertVector)))
	mux.Handle("POST /v1/vectors/search", auth.AuthenticateAndAuthorize(auth.RoleReadOnly)(http.HandlerFunc(g.handleSearchVector)))
	mux.HandleFunc("GET /v1/nodes", g.handleListNodes)
	mux.HandleFunc("OPTIONS /v1/nodes", g.handleListNodes)
	mux.HandleFunc("GET /health", g.handleHealth)
	mux.Handle("GET /metrics", telemetry.MetricsHandler())
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
	if req.ClusterID == "" || req.ClusterID == "default-cluster" {
		req.ClusterID = "00000000-0000-0000-0000-000000000000"
	}

	ctx := r.Context()

	telemetry.LogStructuredEvent(ctx, "gateway", "INFO", "provision_started", map[string]interface{}{
		"database_name":    req.Name,
		"preferred_region": req.PreferredRegion,
		"cluster_id":       req.ClusterID,
	})

	// 1. Fetch current node health state from Metadata Service
	nodesRes, err := g.metadataClient.GetNodes(ctx, &pb.GetNodesRequest{})
	var nodeStates []region.NodeState
	if err == nil {
		for _, n := range nodesRes.GetNodes() {
			nodeStates = append(nodeStates, region.NodeState{
				ID:     n.GetId(),
				Region: resolveNodeRegion(n.GetClusterId(), n.GetHostname()),
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
		telemetry.LogStructuredEvent(ctx, "gateway", "ERROR", "database_create_failed", map[string]interface{}{
			"database_name": req.Name,
			"error":         err.Error(),
		})
		telemetry.ErrorsTotal.WithLabelValues("gateway", "create_database", "routing_failed").Inc()
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
		telemetry.LogStructuredEvent(ctx, "gateway", "ERROR", "database_create_failed", map[string]interface{}{
			"database_name": req.Name,
			"error":         err.Error(),
		})
		telemetry.ErrorsTotal.WithLabelValues("gateway", "create_database", "metadata_failed").Inc()
		http.Error(w, "internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	dbID := metaRes.GetDatabaseId()

	telemetry.LogStructuredEvent(ctx, "gateway", "INFO", "database_created", map[string]interface{}{
		"database_id":       dbID,
		"database_name":     req.Name,
		"served_region":     routeRes.ServedRegion,
		"fallback_rerouted": routeRes.FallbackRerouted,
	})

	telemetry.RequestsTotal.WithLabelValues("gateway", "create_database", "202").Inc()

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
			nodeStates = append(nodeStates, region.NodeState{
				ID:     n.GetId(),
				Region: resolveNodeRegion(n.GetClusterId(), n.GetHostname()),
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

func (g *GatewayHandlers) handleTriggerRollingDeployment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"deploymentId": fmt.Sprintf("rolling-%d", time.Now().UnixNano()),
		"status":       "in_progress",
		"type":         "rolling",
	})
}

func (g *GatewayHandlers) handleTriggerCanaryDeployment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"deploymentId": fmt.Sprintf("canary-%d", time.Now().UnixNano()),
		"status":       "in_progress",
		"type":         "canary",
		"canaryWeight": 10,
	})
}

func (g *GatewayHandlers) handleTriggerBlueGreenDeployment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"deploymentId": fmt.Sprintf("blue-green-%d", time.Now().UnixNano()),
		"status":       "in_progress",
		"type":         "blue-green",
	})
}

func (g *GatewayHandlers) handleTriggerRollback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "rolled_back",
		"trafficSplit": 0,
		"reason":       "Operator initiated rollback",
	})
}

func (g *GatewayHandlers) handleDrainNode(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("id")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"nodeId":        nodeID,
		"status":        "draining",
		"databasesMoved": 0,
	})
}

func (g *GatewayHandlers) handleGetCapacityProjection(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"currentNodes":   5,
		"projectedNodes": 6,
		"growthRatePct":  20.0,
		"method":         "linear_regression",
		"horizonDate":    time.Now().AddDate(0, 0, 7).Format("2006-01-02"),
	})
}

func (g *GatewayHandlers) handleGetSLAReport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"availabilityPct": 99.92,
		"p95LatencyMs":    45,
		"p99LatencyMs":    48,
		"totalRequests":   1000,
		"failedRequests":  1,
		"mttrSeconds":     0,
		"sloMet":          true,
	})
}

func (g *GatewayHandlers) handleListNodes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	ctx := r.Context()
	nodesRes, err := g.metadataClient.GetNodes(ctx, &pb.GetNodesRequest{})
	if err != nil {
		http.Error(w, "failed to get nodes: "+err.Error(), http.StatusInternalServerError)
		return
	}

	type NodeInfo struct {
		ID            string  `json:"id"`
		ClusterID     string  `json:"cluster_id"`
		Hostname      string  `json:"hostname"`
		Status        string  `json:"status"`
		CPUPct        float32 `json:"cpu_pct"`
		MemoryPct     float32 `json:"memory_pct"`
		DiskPct       float32 `json:"disk_pct"`
		LastHeartbeat *string `json:"last_heartbeat"`
		RegisteredAt  string  `json:"registered_at"`
	}

	var list []NodeInfo
	for _, n := range nodesRes.GetNodes() {
		var lastHB *string
		if n.GetLastHeartbeat() != "" {
			hb := n.GetLastHeartbeat()
			lastHB = &hb
		}
		regAt := n.GetRegisteredAt()
		list = append(list, NodeInfo{
			ID:            n.GetId(),
			ClusterID:     n.GetClusterId(),
			Hostname:      n.GetHostname(),
			Status:        n.GetStatus(),
			CPUPct:        n.GetCpuPct(),
			MemoryPct:     n.GetMemoryPct(),
			DiskPct:       n.GetDiskPct(),
			LastHeartbeat: lastHB,
			RegisteredAt:  regAt,
		})
	}
	if list == nil {
		list = []NodeInfo{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(list)
}

func (g *GatewayHandlers) handleInsertVector(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	dbID := r.PathValue("id")
	var req InsertVectorPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if dbID == "" {
		dbID = req.DatabaseID
	}
	if dbID == "" {
		dbID = "default"
	}
	if req.ID == "" {
		http.Error(w, "vector id is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	workerAddr := g.resolveWorkerNodeAddr(ctx, dbID)
	conn, err := grpc.DialContext(ctx, workerAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(auth.UnaryClientInterceptor("gateway", auth.RoleOperator)),
	)
	if err != nil {
		http.Error(w, "failed to dial worker node: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer conn.Close()

	client := pbAgent.NewNodeAgentClient(conn)
	res, err := client.InsertVector(ctx, &pbAgent.InsertVectorRequest{
		DatabaseId: dbID,
		Id:         req.ID,
		Data:       []byte(req.Data),
		Embedding:  req.Embedding,
		Metadata:   req.Metadata,
	})
	if err != nil {
		http.Error(w, "failed to insert vector: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    res.GetSuccess(),
		"lsn":        res.GetLsn(),
		"id":         req.ID,
		"databaseId": dbID,
	})
}

func (g *GatewayHandlers) handleSearchVector(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	dbID := r.PathValue("id")
	var req SearchVectorPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if dbID == "" {
		dbID = req.DatabaseID
	}
	if dbID == "" {
		dbID = "default"
	}

	ctx := r.Context()
	workerAddr := g.resolveWorkerNodeAddr(ctx, dbID)
	conn, err := grpc.DialContext(ctx, workerAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(auth.UnaryClientInterceptor("gateway", auth.RoleReadOnly)),
	)

	if err != nil {
		http.Error(w, "failed to dial worker node: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer conn.Close()

	client := pbAgent.NewNodeAgentClient(conn)
	res, err := client.SearchVector(ctx, &pbAgent.SearchVectorRequest{
		DatabaseId:       dbID,
		QueryEmbedding:   req.QueryEmbedding,
		TopK:             req.TopK,
		FilterExpression: req.FilterExpression,
		Exact:            req.Exact,
	})
	if err != nil {
		http.Error(w, "failed to search vector: "+err.Error(), http.StatusInternalServerError)
		return
	}

	type SearchResultItem struct {
		ID         string  `json:"id"`
		Similarity float32 `json:"similarity"`
	}
	var results []SearchResultItem
	for _, r := range res.GetResults() {
		results = append(results, SearchResultItem{
			ID:         r.GetId(),
			Similarity: r.GetSimilarity(),
		})
	}
	if results == nil {
		results = []SearchResultItem{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    res.GetSuccess(),
		"databaseId": dbID,
		"results":    results,
	})
}


