package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/onlyarnav/nimbusdb/services/gateway/handlers"
	pb "github.com/onlyarnav/nimbusdb/services/metadata-service/proto"
)

func main() {
	metaAddr := os.Getenv("METADATA_SERVICE_ADDR")
	if metaAddr == "" {
		metaAddr = "localhost:50051"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	slog.Info("starting API Gateway service", "metadata_addr", metaAddr, "port", port)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, metaAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Error("failed to connect to Metadata Service", "error", err)
		os.Exit(1)
	}
	defer conn.Close()

	metaClient := pb.NewMetadataServiceClient(conn)
	gh := handlers.NewGatewayHandlers(metaClient)

	mux := http.NewServeMux()
	gh.RegisterRoutes(mux)

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("gateway server error", "error", err)
		}
	}()

	slog.Info("API Gateway server listening", "port", port)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	slog.Info("shutting down API Gateway...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("gateway server shutdown error", "error", err)
	}
}
