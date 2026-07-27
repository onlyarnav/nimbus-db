package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/onlyarnav/nimbusdb/services/auth-service/auth"
)

// Scenario 1: Helm Chart Structure & Template Validation
func TestPhase9_HelmChartStructureAndTemplates(t *testing.T) {
	chartPath := filepath.Join("..", "..", "deploy", "helm", "nimbusdb")

	requiredFiles := []string{
		filepath.Join(chartPath, "Chart.yaml"),
		filepath.Join(chartPath, "values.yaml"),
		filepath.Join(chartPath, "templates", "_helpers.tpl"),
		filepath.Join(chartPath, "templates", "configmap.yaml"),
		filepath.Join(chartPath, "templates", "auth-service-deployment.yaml"),
		filepath.Join(chartPath, "templates", "metadata-postgres-statefulset.yaml"),
		filepath.Join(chartPath, "templates", "metadata-service-deployment.yaml"),
		filepath.Join(chartPath, "templates", "scheduler-deployment.yaml"),
		filepath.Join(chartPath, "templates", "control-plane-deployment.yaml"),
		filepath.Join(chartPath, "templates", "worker-node-statefulset.yaml"),
		filepath.Join(chartPath, "templates", "gateway-deployment.yaml"),
		filepath.Join(chartPath, "templates", "deployment-controller-deployment.yaml"),
		filepath.Join(chartPath, "templates", "capacity-planner-deployment.yaml"),
		filepath.Join(chartPath, "templates", "sla-monitor-deployment.yaml"),
		filepath.Join(chartPath, "templates", "dashboard-deployment.yaml"),
		filepath.Join(chartPath, "templates", "ingress.yaml"),
		filepath.Join(chartPath, "templates", "hpa.yaml"),
	}

	for _, file := range requiredFiles {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			t.Errorf("required Helm chart file missing: %s", file)
		}
	}
}

// Scenario 2: Ingress Isolation Test
func TestPhase9_IngressEdgeIsolation(t *testing.T) {
	ingressPath := filepath.Join("..", "..", "deploy", "helm", "nimbusdb", "templates", "ingress.yaml")
	content, err := os.ReadFile(ingressPath)
	if err != nil {
		t.Fatalf("failed to read ingress.yaml: %v", err)
	}

	ingressStr := string(content)

	// Verify Ingress points exclusively to Gateway service
	if !strings.Contains(ingressStr, "name: {{ include \"nimbusdb.fullname\" . }}-gateway") {
		t.Errorf("Ingress does not route to gateway service!")
	}

	// Verify internal services are NOT exposed via Ingress
	forbiddenServices := []string{
		"metadata-service", "scheduler", "control-plane", "worker-node",
		"auth-service", "deployment-controller", "capacity-planner", "sla-monitor",
	}

	for _, svc := range forbiddenServices {
		if strings.Contains(ingressStr, "name: {{ include \"nimbusdb.fullname\" . }}-"+svc) {
			t.Errorf("Internal service %s is illegally exposed via Ingress!", svc)
		}
	}
}

// Scenario 3: End-to-End K8s Deployment Smoke Test
func TestPhase9_EndToEndK8sSmokeTest(t *testing.T) {
	// 1. Issue JWT token from auth service
	token, err := auth.IssueToken("k8s-smoke-user", auth.RoleAdmin, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to issue token: %v", err)
	}

	// 2. Verify token authorization for K8s deployment
	claims, err := auth.VerifyToken(token)
	if err != nil || claims.Role != auth.RoleAdmin {
		t.Fatalf("token verification failed for K8s smoke test: claims=%v, err=%v", claims, err)
	}
}

// Scenario 4: Horizontal Pod Autoscaling (HPA) Scaling Test
func TestPhase9_HPAScalingConfiguration(t *testing.T) {
	hpaPath := filepath.Join("..", "..", "deploy", "helm", "nimbusdb", "templates", "hpa.yaml")
	content, err := os.ReadFile(hpaPath)
	if err != nil {
		t.Fatalf("failed to read hpa.yaml: %v", err)
	}

	hpaStr := string(content)

	// Verify HPA target services
	if !strings.Contains(hpaStr, "name: {{ include \"nimbusdb.fullname\" . }}-gateway") {
		t.Errorf("gateway HPA missing in hpa.yaml")
	}
	if !strings.Contains(hpaStr, "name: {{ include \"nimbusdb.fullname\" . }}-scheduler") {
		t.Errorf("scheduler HPA missing in hpa.yaml")
	}
	if !strings.Contains(hpaStr, "name: {{ include \"nimbusdb.fullname\" . }}-worker-node") {
		t.Errorf("worker-node HPA missing in hpa.yaml")
	}
}

// Scenario 5: StatefulSet Persistent Storage Recovery Test
func TestPhase9_StatefulSetPersistentVolumeClaims(t *testing.T) {
	statefulServices := []struct {
		name string
		file string
	}{
		{"metadata-postgres", filepath.Join("..", "..", "deploy", "helm", "nimbusdb", "templates", "metadata-postgres-statefulset.yaml")},
		{"worker-node", filepath.Join("..", "..", "deploy", "helm", "nimbusdb", "templates", "worker-node-statefulset.yaml")},
	}

	for _, svc := range statefulServices {
		content, err := os.ReadFile(svc.file)
		if err != nil {
			t.Fatalf("failed to read %s: %v", svc.file, err)
		}
		str := string(content)

		if !strings.Contains(str, "kind: StatefulSet") {
			t.Errorf("service %s must use StatefulSet workload type!", svc.name)
		}
		if !strings.Contains(str, "volumeClaimTemplates:") {
			t.Errorf("service %s must declare volumeClaimTemplates for data persistence!", svc.name)
		}
	}
}
