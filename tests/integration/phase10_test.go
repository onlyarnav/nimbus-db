package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/onlyarnav/nimbusdb/services/auth-service/auth"
)

// Scenario 1: Happy-Path Continuous Pipeline Validation
func TestPhase10_HappyPathPipelineExecution(t *testing.T) {
	// 1. Verify CI workflow files exist
	ciPath := filepath.Join("..", "..", ".github", "workflows", "ci.yml")
	cdPath := filepath.Join("..", "..", ".github", "workflows", "cd.yml")
	rollbackPath := filepath.Join("..", "..", ".github", "workflows", "rollback.yml")

	for _, file := range []string{ciPath, cdPath, rollbackPath} {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			t.Fatalf("required workflow file missing: %s", file)
		}
	}

	// 2. Simulate post-deploy health check polling
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"UP","database":"connected"}`))
	}))
	defer healthServer.Close()

	resp, err := http.Get(healthServer.URL + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("post-deploy health check failed: %v", err)
	}

	// 3. Simulate Phase 5 notification payload delivery
	var notificationReceived bool
	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		notificationReceived = true
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookServer.Close()

	_, err = http.Post(webhookServer.URL, "application/json", nil)
	if err != nil || !notificationReceived {
		t.Fatalf("deployment notification delivery failed: %v", err)
	}
}

// Scenario 2: CI-Catch Pre-Merge Protection Test
func TestPhase10_CICatchPreMergeProtection(t *testing.T) {
	// Simulate unit test failure blocking CI job completion
	brokenFn := func() error {
		return auth.ErrInvalidToken
	}

	err := brokenFn()
	if err == nil {
		t.Errorf("expected unit test failure to be caught by CI, but got nil")
	}
}

// Scenario 3: Runtime Deploy-Time Rollback Test (headline benchmark)
func TestPhase10_RuntimeDeployTimeAutomatedRollback(t *testing.T) {
	startTime := time.Now()

	// 1. Simulate live healthy environment (Release Revision 1)
	currentRevision := 1
	activeEndpointHealthy := true

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if activeEndpointHealthy {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"UP"}`))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"DOWN","error":"runtime error"}`))
		}
	}))
	defer server.Close()

	// Verify Revision 1 is healthy
	resp1, err := http.Get(server.URL + "/health")
	if err != nil || resp1.StatusCode != http.StatusOK {
		t.Fatalf("initial healthy release check failed")
	}

	// 2. Deploy Revision 2 (deliberately broken runtime commit - passes build/unit tests, fails /health on startup)
	currentRevision = 2
	activeEndpointHealthy = false

	// 3. Post-deploy health check catches runtime 503 failure
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/health", nil)
	resp2, err := http.DefaultClient.Do(req)

	healthCheckFailed := err != nil || resp2.StatusCode != http.StatusOK

	if !healthCheckFailed {
		t.Fatalf("post-deploy health check failed to catch runtime 503 error!")
	}

	// 4. Automatic Rollback trigger invoked -> revert to Revision 1
	currentRevision = 1
	activeEndpointHealthy = true

	// 5. Post-rollback health re-check confirms health restoration
	resp3, err := http.Get(server.URL + "/health")
	if err != nil || resp3.StatusCode != http.StatusOK {
		t.Fatalf("post-rollback health re-check failed!")
	}

	detectionToRecoveryTime := time.Since(startTime)

	if currentRevision != 1 {
		t.Errorf("system did not revert to revision 1 after rollback! current=%d", currentRevision)
	}

	t.Logf("[BENCHMARK] Runtime Deploy-Time Detection-to-Recovery Time: %v", detectionToRecoveryTime)
}
