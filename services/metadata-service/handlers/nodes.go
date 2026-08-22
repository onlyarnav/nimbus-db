package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NodeResponse defines the JSON schema for node information returned to the dashboard.
type NodeResponse struct {
	ID            string  `json:"id"`
	ClusterID     string  `json:"cluster_id"`
	Hostname      string  `json:"hostname"`
	Region        string  `json:"region"`
	Status        string  `json:"status"`
	CPUPct        float32 `json:"cpu_pct"`
	MemoryPct     float32 `json:"memory_pct"`
	DiskPct       float32 `json:"disk_pct"`
	LastHeartbeat *string `json:"last_heartbeat"`
	RegisteredAt  string  `json:"registered_at"`
}

// NodesHandler returns the list of all registered nodes in the cluster.
func NodesHandler(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = w.Write([]byte(`{"error": "method not allowed"}`))
			return
		}

		ctx := r.Context()
		query := `SELECT n.id, n.cluster_id, n.hostname, n.status, COALESCE(n.cpu_pct, 0), COALESCE(n.memory_pct, 0), COALESCE(n.disk_pct, 0), n.last_heartbeat, n.registered_at, COALESCE(r.name, 'india')
		          FROM nodes n
		          LEFT JOIN clusters c ON n.cluster_id = c.id
		          LEFT JOIN regions r ON c.region_id = r.id`
		rows, err := db.Query(ctx, query)
		if err != nil {
			slog.ErrorContext(ctx, "failed to query nodes", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error": "failed to query nodes"}`))
			return
		}
		defer rows.Close()

		var list []NodeResponse
		for rows.Next() {
			var n NodeResponse
			var lastHB *time.Time
			var regAt time.Time
			var regName string
			err := rows.Scan(&n.ID, &n.ClusterID, &n.Hostname, &n.Status, &n.CPUPct, &n.MemoryPct, &n.DiskPct, &lastHB, &regAt, &regName)
			if err != nil {
				slog.ErrorContext(ctx, "failed to scan node", "error", err)
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error": "failed to scan node"}`))
				return
			}
			n.Region = regName

			n.RegisteredAt = regAt.Format(time.RFC3339)
			if lastHB != nil {
				formatted := lastHB.Format(time.RFC3339)
				n.LastHeartbeat = &formatted
			}
			list = append(list, n)
		}

		if list == nil {
			list = []NodeResponse{}
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(list)
	}
}
