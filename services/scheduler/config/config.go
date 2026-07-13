package config

import (
	"os"
)

// Config holds the Scheduler configuration.
type Config struct {
	MetadataGRPCAddr string
	SchedulerPort    string
}

// Load loads configurations from environment variables.
func Load() *Config {
	metaAddr := os.Getenv("METADATA_GRPC_ADDR")
	if metaAddr == "" {
		metaAddr = "localhost:50051"
	}
	port := os.Getenv("SCHEDULER_PORT")
	if port == "" {
		port = "50052"
	}
	return &Config{
		MetadataGRPCAddr: metaAddr,
		SchedulerPort:    port,
	}
}
