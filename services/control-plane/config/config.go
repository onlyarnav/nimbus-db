package config

import (
	"os"
)

// Config stores control plane configurations.
type Config struct {
	HTTPPort          string
	MetadataGRPCAddr  string
	SchedulerGRPCAddr string
}

// Load loads configurations from environment variables.
func Load() *Config {
	port := os.Getenv("HTTP_PORT")
	if port == "" {
		port = "8082"
	}
	metadataAddr := os.Getenv("METADATA_GRPC_ADDR")
	if metadataAddr == "" {
		metadataAddr = "localhost:50051"
	}
	schedulerAddr := os.Getenv("SCHEDULER_GRPC_ADDR")
	if schedulerAddr == "" {
		schedulerAddr = "localhost:50052"
	}
	return &Config{
		HTTPPort:          port,
		MetadataGRPCAddr:  metadataAddr,
		SchedulerGRPCAddr: schedulerAddr,
	}
}
