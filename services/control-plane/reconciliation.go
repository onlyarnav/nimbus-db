package main

import (
	"context"
	"log/slog"
	"time"

	pb "github.com/onlyarnav/nimbusdb/services/control-plane/proto/metadata"
)

// Reconciler checks and resolves database placement inconsistencies.
type Reconciler struct {
	metadataClient pb.MetadataServiceClient
	orchestrator   *Orchestrator
	timeout        time.Duration
}

// NewReconciler creates a new Reconciler instance.
func NewReconciler(mc pb.MetadataServiceClient, orch *Orchestrator, timeout time.Duration) *Reconciler {
	return &Reconciler{
		metadataClient: mc,
		orchestrator:   orch,
		timeout:        timeout,
	}
}

// Start runs the periodic background checks.
func (r *Reconciler) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	slog.Info("starting Control Plane background reconciler", "interval", interval, "timeout", r.timeout)

	for {
		select {
		case <-ctx.Done():
			slog.Info("stopping reconciler loop")
			return
		case <-ticker.C:
			r.Reconcile(ctx)
		}
	}
}

// Reconcile sweeps for databases stuck in 'provisioning' past the timeout parameter.
func (r *Reconciler) Reconcile(ctx context.Context) {
	res, err := r.metadataClient.ListDatabases(ctx, &pb.ListDatabasesRequest{})
	if err != nil {
		slog.Error("reconciler failed to list databases from metadata", "error", err)
		return
	}

	for _, db := range res.GetDatabases() {
		if db.GetStatus() != "provisioning" {
			continue
		}

		updatedAt, err := time.Parse(time.RFC3339, db.GetUpdatedAt())
		if err != nil {
			slog.Error("reconciler failed to parse database updated_at timestamp", "database_id", db.GetId(), "updated_at", db.GetUpdatedAt(), "error", err)
			continue
		}

		if time.Since(updatedAt) > r.timeout {
			slog.Warn("found database stuck in provisioning state past timeout threshold, resuming provisioning",
				"database_id", db.GetId(), "name", db.GetName(), "updated_at", db.GetUpdatedAt(), "elapsed", time.Since(updatedAt),
			)

			// Update the database update time first by sending a status touch.
			// This prevents duplicate triggers in subsequent reconciliation loop ticks.
			_, err = r.metadataClient.UpdateDatabaseStatus(ctx, &pb.UpdateDatabaseStatusRequest{
				DatabaseId: db.GetId(),
				Status:     "provisioning",
				Attempts:   db.GetAttempts(), // Keep current attempt
			})
			if err != nil {
				slog.Error("reconciler failed to touch database record time", "database_id", db.GetId(), "error", err)
				continue
			}

			// Resume provisioning flow in background
			go r.orchestrator.ProvisionDatabase(context.Background(), db.GetId(), db.GetName(), db.GetClusterId())
		}
	}
}
