package config

import (
	"os"
)

// Config holds the application configuration.
type Config struct {
	DatabaseURL string
	Port        string
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
	return &Config{
		DatabaseURL: dbURL,
		Port:        port,
	}
}
