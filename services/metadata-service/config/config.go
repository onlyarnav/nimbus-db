package config

import (
	"os"
)

// Config holds the application configuration.
type Config struct {
	DatabaseURL string
	Port        string
	GRPCPort    string
}

// Load loads the configuration from environment variables with sensible defaults.
func Load() *Config {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/nimbusdb?sslmode=disable"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	grpcPort := os.Getenv("GRPC_PORT")
	if grpcPort == "" {
		grpcPort = "50051"
	}
	return &Config{
		DatabaseURL: dbURL,
		Port:        port,
		GRPCPort:    grpcPort,
	}
}
