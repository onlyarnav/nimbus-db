package integration

import (
	"context"
	"net/http"
	"os/exec"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/onlyarnav/nimbusdb/tests/integration/proto"
)

func TestClusterIntegration(t *testing.T) {
	// Check if docker daemon is running
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Skipping integration test: docker daemon is not running")
	}

	// 1. Spin up the docker-compose stack
	t.Log("Starting docker-compose services...")
	cmdUp := exec.Command("docker", "compose", "-f", "../../deploy/docker/docker-compose.yml", "up", "-d", "--build")
	if out, err := cmdUp.CombinedOutput(); err != nil {
		t.Fatalf("failed to start docker-compose: %v\nOutput: %s", err, string(out))
	}
	defer func() {
		t.Log("Cleaning up docker-compose services...")
		cmdDown := exec.Command("docker", "compose", "-f", "../../deploy/docker/docker-compose.yml", "down", "-v")
		_ = cmdDown.Run()
	}()

	// 2. Poll health check of Metadata Service HTTP endpoint
	t.Log("Waiting for Metadata Service to be healthy...")
	metaHealthy := false
	for i := 0; i < 45; i++ {
		resp, err := http.Get("http://localhost:8080/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			metaHealthy = true
			resp.Body.Close()
			break
		}
		time.Sleep(1 * time.Second)
	}
	if !metaHealthy {
		t.Fatal("Metadata Service failed to become healthy on port 8080")
	}
	t.Log("Metadata Service is healthy.")

	// 3. Dial Metadata and Scheduler services
	ctx := context.Background()
	metaConn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to dial metadata gRPC: %v", err)
	}
	defer metaConn.Close()
	metaClient := pb.NewMetadataServiceClient(metaConn)

	schedConn, err := grpc.Dial("localhost:50052", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to dial scheduler gRPC: %v", err)
	}
	defer schedConn.Close()
	schedClient := pb.NewSchedulerServiceClient(schedConn)

	// 4. Wait for all 3 nodes to register and send their first heartbeat (within 20s)
	t.Log("Waiting for nodes to register and send first heartbeat...")
	var worker2ID string
	var registeredNodes []*pb.NodeInfo

	for i := 0; i < 25; i++ {
		res, err := metaClient.GetNodes(ctx, &pb.GetNodesRequest{})
		if err == nil && len(res.GetNodes()) == 3 {
			allReady := true
			for _, n := range res.GetNodes() {
				if n.Status != "healthy" || n.LastHeartbeat == "" {
					allReady = false
				}
				if n.Hostname == "worker-2" {
					worker2ID = n.Id
				}
			}
			if allReady {
				registeredNodes = res.GetNodes()
				break
			}
		}
		time.Sleep(1 * time.Second)
	}

	if len(registeredNodes) != 3 {
		t.Fatalf("expected 3 healthy nodes with active heartbeats, registered: %d", len(registeredNodes))
	}
	t.Log("All 3 nodes are registered and sending heartbeats.")

	// 5. Pause worker-2 heartbeat via debug endpoint
	t.Log("Pausing worker-2 heartbeats...")
	t0 := time.Now()
	resp, err := http.Post("http://localhost:8082/debug/pause", "application/json", nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("failed to pause worker-2: %v", err)
	}
	resp.Body.Close()

	// 6. Assert transitions: healthy -> unhealthy (15-60s) -> dead (60s+)
	t.Log("Monitoring worker-2 state transitions...")
	unhealthyObserved := false
	deadObserved := false

	timeout := time.After(85 * time.Second)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

Loop:
	for {
		select {
		case <-timeout:
			t.Fatal("timed out waiting for worker-2 to transition to dead status")
		case <-ticker.C:
			res, err := metaClient.GetNodes(ctx, &pb.GetNodesRequest{})
			if err != nil {
				t.Logf("GetNodes query failed: %v", err)
				continue
			}

			var status string
			for _, n := range res.GetNodes() {
				if n.Hostname == "worker-2" {
					status = n.Status
				}
			}

			elapsed := time.Since(t0)
			t.Logf("Elapsed: %v, worker-2 status: %s", elapsed.Round(time.Second), status)

			if status == "unhealthy" && !unhealthyObserved {
				unhealthyObserved = true
				t.Logf("worker-2 transitioned to UNHEALTHY after %v", elapsed.Round(time.Second))
				if elapsed < 12*time.Second || elapsed > 25*time.Second {
					t.Errorf("worker-2 marked unhealthy out of expected bounds: elapsed=%v", elapsed)
				}
			}

			if status == "dead" {
				deadObserved = true
				t.Logf("worker-2 transitioned to DEAD after %v", elapsed.Round(time.Second))
				if elapsed < 55*time.Second || elapsed > 75*time.Second {
					t.Errorf("worker-2 marked dead out of expected bounds: elapsed=%v", elapsed)
				}
				break Loop
			}
		}
	}

	if !unhealthyObserved || !deadObserved {
		t.Fatalf("failed to observe both unhealthy and dead transitions: unhealthy=%t, dead=%t", unhealthyObserved, deadObserved)
	}

	// 7. Verify Scheduler never schedules worker-2
	t.Log("Requesting scheduling decision...")
	for i := 0; i < 5; i++ {
		schedRes, err := schedClient.Schedule(ctx, &pb.ScheduleRequest{ClusterId: "00000000-0000-0000-0000-000000000000"})
		if err != nil {
			t.Fatalf("scheduler request failed: %v", err)
		}
		t.Logf("Scheduler chose node ID: %s, Score: %f", schedRes.GetNodeId(), schedRes.GetScore())
		if schedRes.GetNodeId() == worker2ID {
			t.Fatal("scheduler selected worker-2 even though it is dead!")
		}
	}
	t.Log("Scheduler successfully avoided dead node.")

	// 8. Resume worker-2 heartbeats
	t.Log("Resuming worker-2 heartbeats...")
	resp, err = http.Post("http://localhost:8082/debug/resume", "application/json", nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("failed to resume worker-2: %v", err)
	}
	resp.Body.Close()

	// 9. Verify worker-2 recovers to healthy
	t.Log("Waiting for worker-2 to return to healthy...")
	recovered := false
	for i := 0; i < 15; i++ {
		res, err := metaClient.GetNodes(ctx, &pb.GetNodesRequest{})
		if err == nil {
			for _, n := range res.GetNodes() {
				if n.Hostname == "worker-2" && n.Status == "healthy" {
					recovered = true
					break
				}
			}
		}
		if recovered {
			break
		}
		time.Sleep(1 * time.Second)
	}

	if !recovered {
		t.Fatal("worker-2 failed to return to healthy after heartbeats resumed")
	}
	t.Log("worker-2 recovered successfully. E2E cluster behavior is correct!")
}
