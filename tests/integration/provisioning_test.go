package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/onlyarnav/nimbusdb/tests/integration/proto"
)

type CreateDatabaseRequest struct {
	Name      string `json:"name"`
	ClusterID string `json:"clusterId"`
}

type CreateDatabaseResponse struct {
	DatabaseID string `json:"databaseId"`
	Status     string `json:"status"`
}

type DatabaseResponse struct {
	DatabaseID string `json:"databaseId"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	NodeID     string `json:"nodeId"`
	Endpoint   string `json:"endpoint"`
	Attempts   int32  `json:"attempts"`
	CreatedAt  string `json:"createdAt"`
}

func TestDatabaseProvisioningIntegration(t *testing.T) {
	// Skip if docker daemon is not running
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Skipping integration test: docker daemon is not running")
	}

	t.Log("Starting docker-compose services...")
	// Force rebuilding of control-plane and worker-node
	cmdUp := exec.Command("docker", "compose", "-f", "../../deploy/docker/docker-compose.yml", "up", "-d", "--build")
	if out, err := cmdUp.CombinedOutput(); err != nil {
		t.Fatalf("failed to start docker-compose: %v\nOutput: %s", err, string(out))
	}
	defer func() {
		t.Log("Cleaning up docker-compose services...")
		cmdDown := exec.Command("docker", "compose", "-f", "../../deploy/docker/docker-compose.yml", "down", "-v")
		_ = cmdDown.Run()
	}()

	t.Log("Waiting for Services to be healthy...")
	time.Sleep(10 * time.Second) // wait for bootstrap

	// 1. Happy Path
	t.Run("HappyPath", func(t *testing.T) {
		req := CreateDatabaseRequest{
			Name:      "happy-db",
			ClusterID: "00000000-0000-0000-0000-000000000000",
		}
		body, _ := json.Marshal(req)
		resp, err := http.Post("http://localhost:8085/v1/databases", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("failed to post database creation: %v", err)
		}
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("expected 202 Accepted, got %d", resp.StatusCode)
		}
		var createResp CreateDatabaseResponse
		_ = json.NewDecoder(resp.Body).Decode(&createResp)
		resp.Body.Close()

		// Poll status
		active := false
		var dbDetails DatabaseResponse
		for i := 0; i < 20; i++ {
			getResp, err := http.Get("http://localhost:8085/v1/databases/" + createResp.DatabaseID)
			if err == nil && getResp.StatusCode == http.StatusOK {
				_ = json.NewDecoder(getResp.Body).Decode(&dbDetails)
				getResp.Body.Close()
				if dbDetails.Status == "active" {
					active = true
					break
				}
			}
			time.Sleep(1 * time.Second)
		}

		if !active {
			t.Fatalf("database failed to become active, current status: %s", dbDetails.Status)
		}
		if dbDetails.NodeID == "" || dbDetails.Endpoint == "" {
			t.Errorf("expected node ID and endpoint to be populated, got details: %+v", dbDetails)
		}
	})

	// 2. Retry Path
	t.Run("RetryPath", func(t *testing.T) {
		// Dial scheduler to find which node it will pick first
		conn, err := grpc.Dial("localhost:50052", grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.Fatalf("failed to dial scheduler: %v", err)
		}
		defer conn.Close()
		schedClient := pb.NewSchedulerServiceClient(conn)
		res, err := schedClient.Schedule(context.Background(), &pb.ScheduleRequest{ClusterId: "00000000-0000-0000-0000-000000000000"})
		if err != nil {
			t.Fatalf("scheduler failed: %v", err)
		}

		nodeID := res.GetNodeId()

		// Get node details to find hostname
		connMeta, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.Fatalf("failed to dial metadata: %v", err)
		}
		defer connMeta.Close()
		metaClient := pb.NewMetadataServiceClient(connMeta)
		nodesRes, err := metaClient.GetNodes(context.Background(), &pb.GetNodesRequest{})
		if err != nil {
			t.Fatalf("failed to get nodes: %v", err)
		}

		var hostname string
		for _, n := range nodesRes.GetNodes() {
			if n.Id == nodeID {
				hostname = n.Hostname
			}
		}

		// Map worker hostname to its host debug port
		debugPort := ""
		switch hostname {
		case "worker-1":
			debugPort = "8081"
		case "worker-2":
			debugPort = "8082"
		case "worker-3":
			debugPort = "8083"
		}

		if debugPort == "" {
			t.Fatalf("unknown hostname %q", hostname)
		}

		t.Logf("Injecting failure on selected node %s (port %s)", hostname, debugPort)
		injectResp, err := http.Post(fmt.Sprintf("http://localhost:%s/debug/inject-failure?attempts=1", debugPort), "application/json", nil)
		if err != nil || injectResp.StatusCode != http.StatusOK {
			t.Fatalf("failed to inject failure: %v", err)
		}
		injectResp.Body.Close()

		// Create database
		req := CreateDatabaseRequest{
			Name:      "retry-db",
			ClusterID: "00000000-0000-0000-0000-000000000000",
		}
		body, _ := json.Marshal(req)
		resp, err := http.Post("http://localhost:8085/v1/databases", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		var createResp CreateDatabaseResponse
		_ = json.NewDecoder(resp.Body).Decode(&createResp)
		resp.Body.Close()

		// Poll status
		active := false
		var dbDetails DatabaseResponse
		for i := 0; i < 20; i++ {
			getResp, err := http.Get("http://localhost:8085/v1/databases/" + createResp.DatabaseID)
			if err == nil && getResp.StatusCode == http.StatusOK {
				_ = json.NewDecoder(getResp.Body).Decode(&dbDetails)
				getResp.Body.Close()
				if dbDetails.Status == "active" {
					active = true
					break
				}
			}
			time.Sleep(1 * time.Second)
		}

		if !active {
			t.Fatalf("database failed to become active, status: %s", dbDetails.Status)
		}
		if dbDetails.NodeID == nodeID {
			t.Errorf("expected database to be provisioned on a different node than %s", nodeID)
		}
		if dbDetails.Attempts != 2 {
			t.Errorf("expected exactly 2 attempts, got %d", dbDetails.Attempts)
		}
	})

	// 3. Exhausted Retries
	t.Run("ExhaustedRetries", func(t *testing.T) {
		// Inject failure on all three worker nodes
		for _, port := range []string{"8081", "8082", "8083"} {
			resp, err := http.Post(fmt.Sprintf("http://localhost:%s/debug/inject-failure?attempts=3", port), "application/json", nil)
			if err == nil {
				resp.Body.Close()
			}
		}

		req := CreateDatabaseRequest{
			Name:      "exhaust-db",
			ClusterID: "00000000-0000-0000-0000-000000000000",
		}
		body, _ := json.Marshal(req)
		resp, err := http.Post("http://localhost:8085/v1/databases", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		var createResp CreateDatabaseResponse
		_ = json.NewDecoder(resp.Body).Decode(&createResp)
		resp.Body.Close()

		// Poll status
		failed := false
		var dbDetails DatabaseResponse
		for i := 0; i < 20; i++ {
			getResp, err := http.Get("http://localhost:8085/v1/databases/" + createResp.DatabaseID)
			if err == nil && getResp.StatusCode == http.StatusOK {
				_ = json.NewDecoder(getResp.Body).Decode(&dbDetails)
				getResp.Body.Close()
				if dbDetails.Status == "failed" {
					failed = true
					break
				}
			}
			time.Sleep(1 * time.Second)
		}

		if !failed {
			t.Fatalf("expected database status 'failed', got %q", dbDetails.Status)
		}
		if dbDetails.Attempts != 3 {
			t.Errorf("expected 3 attempts, got %d", dbDetails.Attempts)
		}
	})

	// 4. Crash-recovery reconciliation
	t.Run("CrashRecoveryReconciliation", func(t *testing.T) {
		// Inject hang on all three worker nodes
		for _, port := range []string{"8081", "8082", "8083"} {
			resp, err := http.Post(fmt.Sprintf("http://localhost:%s/debug/inject-failure?hang=1", port), "application/json", nil)
			if err == nil {
				resp.Body.Close()
			}
		}

		req := CreateDatabaseRequest{
			Name:      "crash-db",
			ClusterID: "00000000-0000-0000-0000-000000000000",
		}
		body, _ := json.Marshal(req)
		resp, err := http.Post("http://localhost:8085/v1/databases", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		var createResp CreateDatabaseResponse
		_ = json.NewDecoder(resp.Body).Decode(&createResp)
		resp.Body.Close()

		// Give it a second to start provisioning, then kill the Control Plane container
		time.Sleep(1 * time.Second)
		t.Log("Killing Control Plane container mid-provision...")
		cmdKill := exec.Command("docker", "stop", "nimbusdb_control_plane")
		_ = cmdKill.Run()

		// Verify database remains stuck in provisioning state
		connMeta, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.Fatalf("failed to dial metadata: %v", err)
		}
		metaClient := pb.NewMetadataServiceClient(connMeta)
		dbInfo, err := metaClient.GetDatabase(context.Background(), &pb.GetDatabaseRequest{DatabaseId: createResp.DatabaseID})
		if err != nil {
			connMeta.Close()
			t.Fatalf("failed to get database from metadata: %v", err)
		}
		connMeta.Close()

		if dbInfo.GetDatabase().GetStatus() != "provisioning" {
			t.Fatalf("expected status 'provisioning' before restart, got %q", dbInfo.GetDatabase().GetStatus())
		}

		// Restart Control Plane
		t.Log("Restarting Control Plane container...")
		cmdStart := exec.Command("docker", "start", "nimbusdb_control_plane")
		if err := cmdStart.Run(); err != nil {
			t.Fatalf("failed to restart control plane: %v", err)
		}

		// Wait for reconciliation loop to detect stuck record (timeout is 30 seconds)
		t.Log("Waiting for reconciler to resolve stuck database status...")
		active := false
		var dbDetails DatabaseResponse
		for i := 0; i < 45; i++ {
			getResp, err := http.Get("http://localhost:8085/v1/databases/" + createResp.DatabaseID)
			if err == nil && getResp.StatusCode == http.StatusOK {
				_ = json.NewDecoder(getResp.Body).Decode(&dbDetails)
				getResp.Body.Close()
				if dbDetails.Status == "active" {
					active = true
					break
				}
			}
			time.Sleep(1 * time.Second)
		}

		if !active {
			t.Fatalf("reconciler failed to resolve database to active state, current status: %s", dbDetails.Status)
		}
		t.Log("Reconciler successfully recovered database status!")
	})
}
