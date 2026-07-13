package config

import (
	"os"
)

// Config holds the worker node configuration.
type Config struct {
	MetadataGRPCAddr string
	ClusterID        string
	Hostname         string
	DebugPort        string
}

// Load loads the configuration from environment variables.
func Load() *Config {
	addr := os.Getenv("METADATA_GRPC_ADDR")
	if addr == "" {
		addr = "localhost:50051"
	}
	clusterID := os.Getenv("CLUSTER_ID")
	if clusterID == "" {
		clusterID = "00000000-0000-0000-0000-000000000000"
	}
	hostname := os.Getenv("HOSTNAME")
	if hostname == "" {
		hostname = "worker-local"
	}
	debugPort := os.Getenv("DEBUG_PORT")
	if debugPort == "" {
		debugPort = "8081"
	}
	return &Config{
		MetadataGRPCAddr: addr,
		ClusterID:        clusterID,
		Hostname:         hostname,
		DebugPort:        debugPort,
	}
}
