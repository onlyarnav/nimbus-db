package telemetry

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// RequestDurationSeconds measures HTTP/gRPC request duration distribution.
	RequestDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "request_duration_seconds",
			Help:    "Latency histogram of HTTP and gRPC requests in seconds.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0},
		},
		[]string{"service", "method", "code"},
	)

	// RequestsTotal counts total API requests processed.
	RequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "requests_total",
			Help: "Total count of requests processed by service.",
		},
		[]string{"service", "method", "status"},
	)

	// ErrorsTotal counts total error events labeled by error type.
	ErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "errors_total",
			Help: "Total count of errors categorized by error type.",
		},
		[]string{"service", "method", "error_type"},
	)

	// ActiveConnections tracks current open gateway connections.
	ActiveConnections = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "active_connections",
			Help: "Number of active client connections on gateway.",
		},
		[]string{"service"},
	)

	// RegionHealth tracks regional cluster health status (0=down, 1=degraded, 2=healthy).
	RegionHealth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "region_health",
			Help: "Aggregate health status per region (0=down, 1=degraded, 2=healthy).",
		},
		[]string{"region"},
	)

	// CPUUsagePercent tracks CPU utilization on worker nodes.
	CPUUsagePercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "cpu_usage_percent",
			Help: "Current CPU usage percentage of worker node.",
		},
		[]string{"node_id", "hostname"},
	)

	// MemoryUsagePercent tracks Memory utilization on worker nodes.
	MemoryUsagePercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "memory_usage_percent",
			Help: "Current Memory usage percentage of worker node.",
		},
		[]string{"node_id", "hostname"},
	)

	// DiskUsagePercent tracks Disk utilization on worker nodes.
	DiskUsagePercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "disk_usage_percent",
			Help: "Current Disk usage percentage of worker node.",
		},
		[]string{"node_id", "hostname"},
	)

	// ReplicationLagSeconds tracks replication streaming lag to follower nodes/regions.
	ReplicationLagSeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "replication_lag_seconds",
			Help: "Current WAL replication lag in seconds.",
		},
		[]string{"database_id", "target_region"},
	)
)

func init() {
	prometheus.MustRegister(
		RequestDurationSeconds,
		RequestsTotal,
		ErrorsTotal,
		ActiveConnections,
		RegionHealth,
		CPUUsagePercent,
		MemoryUsagePercent,
		DiskUsagePercent,
		ReplicationLagSeconds,
	)
}

// MetricsHandler returns the Prometheus HTTP handler for scraping.
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}
