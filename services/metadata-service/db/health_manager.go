package db

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// HealthManager runs a background daemon that periodically scans and classifies node health.
type HealthManager struct {
	db       *pgxpool.Pool
	interval time.Duration
}

// NewHealthManager initializes a new HealthManager instance.
func NewHealthManager(db *pgxpool.Pool, interval time.Duration) *HealthManager {
	return &HealthManager{
		db:       db,
		interval: interval,
	}
}

// Start runs the periodic background classification loop.
func (hm *HealthManager) Start(ctx context.Context) {
	slog.Info("starting health manager background daemon", "interval", hm.interval)
	ticker := time.NewTicker(hm.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("stopping health manager background daemon")
			return
		case <-ticker.C:
			if err := hm.CheckHealth(ctx); err != nil {
				slog.Error("health manager check failed", "error", err)
			}
		}
	}
}

// CheckHealth queries all node states and updates statuses based on heartbeat freshness.
func (hm *HealthManager) CheckHealth(ctx context.Context) error {
	rows, err := hm.db.Query(ctx, "SELECT id, status, last_heartbeat, hostname FROM nodes")
	if err != nil {
		return err
	}
	defer rows.Close()

	type nodeState struct {
		id            string
		status        string
		lastHeartbeat *time.Time
		hostname      string
	}

	var nodes []nodeState
	for rows.Next() {
		var ns nodeState
		if err := rows.Scan(&ns.id, &ns.status, &ns.lastHeartbeat, &ns.hostname); err != nil {
			return err
		}
		nodes = append(nodes, ns)
	}
	rows.Close()

	for _, n := range nodes {
		if n.status == "draining" {
			continue // Manually drained nodes are skipped from auto-health classification
		}

		if n.lastHeartbeat == nil {
			if n.status != "unknown" {
				if err := hm.updateNodeStatus(ctx, n.id, "unknown"); err != nil {
					slog.Error("failed to update node status", "node_id", n.id, "error", err)
				}
			}
			continue
		}

		elapsed := time.Since(*n.lastHeartbeat)

		if elapsed >= 60*time.Second {
			if n.status != "dead" {
				slog.Info("node classified as DEAD", "node_id", n.id, "hostname", n.hostname, "elapsed", elapsed)
				if err := hm.updateNodeStatus(ctx, n.id, "dead"); err != nil {
					slog.Error("failed to update node status", "node_id", n.id, "error", err)
				}
			}
		} else if elapsed >= 15*time.Second {
			if n.status != "unhealthy" {
				slog.Warn("Node Down", "node_id", n.id, "hostname", n.hostname, "elapsed", elapsed)
				if err := hm.updateNodeStatus(ctx, n.id, "unhealthy"); err != nil {
					slog.Error("failed to update node status", "node_id", n.id, "error", err)
				}
			}
		} else {
			// Node is online. Perform overload checks (3 consecutive heartbeats above 90%).
			isOverloaded, err := hm.checkIfOverloaded(ctx, n.id)
			if err != nil {
				slog.Error("failed to check overload status", "node_id", n.id, "error", err)
				continue
			}

			targetStatus := "healthy"
			if isOverloaded {
				targetStatus = "overloaded"
			}

			if n.status != targetStatus {
				slog.Info("node status updated based on load", "node_id", n.id, "hostname", n.hostname, "old_status", n.status, "new_status", targetStatus)
				if err := hm.updateNodeStatus(ctx, n.id, targetStatus); err != nil {
					slog.Error("failed to update node status", "node_id", n.id, "error", err)
				}
			}
		}
	}

	return nil
}

func (hm *HealthManager) updateNodeStatus(ctx context.Context, nodeID string, status string) error {
	_, err := hm.db.Exec(ctx, "UPDATE nodes SET status = $1 WHERE id = $2", status, nodeID)
	return err
}

func (hm *HealthManager) checkIfOverloaded(ctx context.Context, nodeID string) (bool, error) {
	rows, err := hm.db.Query(ctx, "SELECT cpu_pct, memory_pct, disk_pct FROM heartbeats WHERE node_id = $1 ORDER BY received_at DESC LIMIT 3", nodeID)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	count := 0
	overloadCount := 0
	for rows.Next() {
		var cpu, mem, disk float32
		if err := rows.Scan(&cpu, &mem, &disk); err != nil {
			return false, err
		}
		count++
		if cpu > 90.0 || mem > 90.0 || disk > 90.0 {
			overloadCount++
		}
	}

	return count >= 3 && overloadCount == count, nil
}
