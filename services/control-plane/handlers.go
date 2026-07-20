package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/onlyarnav/nimbusdb/services/control-plane/proto/metadata"
	pbAgent "github.com/onlyarnav/nimbusdb/services/control-plane/proto/nodeagent"
)

var nameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

type CreateDatabaseRequest struct {
	Name      string `json:"name"`
	ClusterID string `json:"clusterId"`
}

type CreateDatabaseResponse struct {
	DatabaseID string `json:"databaseId"`
	Status     string `json:"status"`
}

type DatabaseResponse struct {
	DatabaseID string `json:"databaseId"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	NodeID     string `json:"nodeId,omitempty"`
	Endpoint   string `json:"endpoint,omitempty"`
	Attempts   int32  `json:"attempts"`
	CreatedAt  string `json:"createdAt"`
}

type Handlers struct {
	metadataClient pb.MetadataServiceClient
	orchestrator   *Orchestrator
}

func NewHandlers(mc pb.MetadataServiceClient, orch *Orchestrator) *Handlers {
	return &Handlers{
		metadataClient: mc,
		orchestrator:   orch,
	}
}

// RegisterRoutes sets up all REST paths on the serve mux.
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/databases", h.handleCreateDatabase)
	mux.HandleFunc("GET /v1/databases/{id}", h.handleGetDatabase)
	mux.HandleFunc("GET /v1/databases", h.handleListDatabases)
	mux.HandleFunc("DELETE /v1/databases/{id}", h.handleDeleteDatabase)
	mux.HandleFunc("GET /health", h.handleHealth)
}

func (h *Handlers) handleCreateDatabase(w http.ResponseWriter, r *http.Request) {
	var req CreateDatabaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// 1. Validation
	if req.Name == "" {
		http.Error(w, "database name is required", http.StatusBadRequest)
		return
	}
	if !nameRegex.MatchString(req.Name) {
		http.Error(w, "invalid database name (must be alphanumeric, dash, or underscore)", http.StatusBadRequest)
		return
	}
	if req.ClusterID == "" {
		http.Error(w, "clusterId is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	slog.InfoContext(ctx, "received create database request", "name", req.Name, "cluster_id", req.ClusterID)

	// 2. Insert provisioning record in Metadata Service to claim name and get database ID
	metaRes, err := h.metadataClient.CreateDatabaseRecord(ctx, &pb.CreateDatabaseRecordRequest{
		Name:      req.Name,
		ClusterId: req.ClusterID,
		Status:    "provisioning",
		Attempts:  1,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to register provisioning database in metadata", "error", err)
		http.Error(w, "internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	dbID := metaRes.GetDatabaseId()

	// 3. Dispatch background thread for provisioning state machine
	go func() {
		h.orchestrator.ProvisionDatabase(context.Background(), dbID, req.Name, req.ClusterID)
	}()

	// 4. Return 202 Accepted
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(CreateDatabaseResponse{
		DatabaseID: dbID,
		Status:     "provisioning",
	})
}

func (h *Handlers) handleGetDatabase(w http.ResponseWriter, r *http.Request) {
	dbID := r.PathValue("id")
	if dbID == "" {
		http.Error(w, "database id is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	metaRes, err := h.metadataClient.GetDatabase(ctx, &pb.GetDatabaseRequest{DatabaseId: dbID})
	if err != nil {
		slog.ErrorContext(ctx, "failed to fetch database details from metadata", "database_id", dbID, "error", err)
		http.Error(w, "database not found", http.StatusNotFound)
		return
	}

	db := metaRes.GetDatabase()
	resp := DatabaseResponse{
		DatabaseID: db.GetId(),
		Name:       db.GetName(),
		Status:     db.GetStatus(),
		NodeID:     db.GetNodeId(),
		Endpoint:   db.GetEndpoint(),
		Attempts:   db.GetAttempts(),
		CreatedAt:  db.GetCreatedAt(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handlers) handleListDatabases(w http.ResponseWriter, r *http.Request) {
	clusterID := r.URL.Query().Get("clusterId")

	ctx := r.Context()
	metaRes, err := h.metadataClient.ListDatabases(ctx, &pb.ListDatabasesRequest{ClusterId: clusterID})
	if err != nil {
		slog.ErrorContext(ctx, "failed to query databases list", "error", err)
		http.Error(w, "internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	dbs := []DatabaseResponse{}
	for _, db := range metaRes.GetDatabases() {
		dbs = append(dbs, DatabaseResponse{
			DatabaseID: db.GetId(),
			Name:       db.GetName(),
			Status:     db.GetStatus(),
			NodeID:     db.GetNodeId(),
			Endpoint:   db.GetEndpoint(),
			Attempts:   db.GetAttempts(),
			CreatedAt:  db.GetCreatedAt(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(dbs)
}

func (h *Handlers) handleDeleteDatabase(w http.ResponseWriter, r *http.Request) {
	dbID := r.PathValue("id")
	if dbID == "" {
		http.Error(w, "database id is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	slog.InfoContext(ctx, "received delete database request", "database_id", dbID)

	// 1. Fetch details to see where database lives
	getRes, err := h.metadataClient.GetDatabase(ctx, &pb.GetDatabaseRequest{DatabaseId: dbID})
	if err != nil {
		slog.ErrorContext(ctx, "failed to delete: database record not found in metadata", "database_id", dbID)
		http.Error(w, "database not found", http.StatusNotFound)
		return
	}

	db := getRes.GetDatabase()

	// 2. Set state to 'deleting' in metadata
	_, err = h.metadataClient.UpdateDatabaseStatus(ctx, &pb.UpdateDatabaseStatusRequest{
		DatabaseId: dbID,
		Status:     "deleting",
	})
	if err != nil {
		slog.WarnContext(ctx, "failed to update deleting status", "database_id", dbID, "error", err)
	}

	// 3. Trigger deletion in background on NodeAgent if node is resolved
	go func() {
		bgCtx := context.Background()
		if db.GetNodeId() != "" {
			// Find hostname
			nodesRes, err := h.metadataClient.GetNodes(bgCtx, &pb.GetNodesRequest{})
			if err == nil {
				var hostname string
				for _, n := range nodesRes.GetNodes() {
					if n.GetId() == db.GetNodeId() {
						hostname = n.GetHostname()
						break
					}
				}
				if hostname != "" {
					var agentAddr string
					if hostname == "worker-local" || hostname == "test-worker-node" || hostname == "localhost" {
						agentAddr = "localhost:50053"
					} else {
						agentAddr = fmt.Sprintf("%s:50053", hostname)
					}
					// Dial and call DeleteDatabase
					delCtx, cancel := context.WithTimeout(bgCtx, 5*time.Second)
					conn, err := grpc.DialContext(delCtx, agentAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
					if err == nil {
						agentClient := pbAgent.NewNodeAgentClient(conn)
						_, _ = agentClient.DeleteDatabase(delCtx, &pbAgent.DeleteDatabaseRequest{DatabaseId: dbID})
						conn.Close()
					}
					cancel()
				}
			}
		}

		// 4. Remove database record completely from registry
		_, err := h.metadataClient.DeleteDatabaseRecord(bgCtx, &pb.DeleteDatabaseRecordRequest{DatabaseId: dbID})
		if err != nil {
			slog.ErrorContext(bgCtx, "failed to delete database record from metadata store", "database_id", dbID, "error", err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *Handlers) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "UP"})
}
