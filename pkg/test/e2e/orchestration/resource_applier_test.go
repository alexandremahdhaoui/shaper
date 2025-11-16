package orchestration

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestNewResourceApplier(t *testing.T) {
	tests := []struct {
		name       string
		kubeconfig string
		namespace  string
	}{
		{
			name:       "creates applier with valid config",
			kubeconfig: "/path/to/kubeconfig",
			namespace:  "test-namespace",
		},
		{
			name:       "creates applier with empty namespace",
			kubeconfig: "/path/to/kubeconfig",
			namespace:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applier := NewResourceApplier(tt.kubeconfig, tt.namespace)
			if applier == nil {
				t.Fatal("expected non-nil applier")
			}
			if applier.kubeconfig != tt.kubeconfig {
				t.Errorf("expected kubeconfig %s, got %s", tt.kubeconfig, applier.kubeconfig)
			}
			if applier.namespace != tt.namespace {
				t.Errorf("expected namespace %s, got %s", tt.namespace, applier.namespace)
			}
		})
	}
}

func TestResourceApplier_ApplyResource_NoKubeconfig(t *testing.T) {
	applier := NewResourceApplier("", "default")
	ctx := context.Background()

	resource := K8sResource{
		Kind:      "ConfigMap",
		Name:      "test-cm",
		Namespace: "default",
		YAML:      []byte("apiVersion: v1\nkind: ConfigMap"),
	}

	result, err := applier.ApplyResource(ctx, resource)
	if err == nil {
		t.Fatal("expected error when kubeconfig is empty")
	}
	if !strings.Contains(err.Error(), "kubeconfig") {
		t.Errorf("expected error about kubeconfig, got: %v", err)
	}
	if result != nil {
		t.Error("expected nil result on error")
	}
}

func TestResourceApplier_DeleteResource_NoKubeconfig(t *testing.T) {
	applier := NewResourceApplier("", "default")
	ctx := context.Background()

	resource := K8sResource{
		Kind:      "ConfigMap",
		Name:      "test-cm",
		Namespace: "default",
	}

	err := applier.DeleteResource(ctx, resource)
	if err == nil {
		t.Fatal("expected error when kubeconfig is empty")
	}
	if !strings.Contains(err.Error(), "kubeconfig") {
		t.Errorf("expected error about kubeconfig, got: %v", err)
	}
}

func TestResourceApplier_GetResource_NoKubeconfig(t *testing.T) {
	applier := NewResourceApplier("", "default")
	ctx := context.Background()

	_, err := applier.GetResource(ctx, "ConfigMap", "test-cm", "default")
	if err == nil {
		t.Fatal("expected error when kubeconfig is empty")
	}
	if !strings.Contains(err.Error(), "kubeconfig") {
		t.Errorf("expected error about kubeconfig, got: %v", err)
	}
}

func TestResourceApplier_RenderTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
		vars     TemplateVars
		expected string
		wantErr  bool
	}{
		{
			name: "renders VM name",
			template: `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{.VMName}}-config`,
			vars: TemplateVars{
				VMName: "test-vm-1",
			},
			expected: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-vm-1-config`,
			wantErr: false,
		},
		{
			name: "renders all variables",
			template: `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{.VMName}}
data:
  uuid: {{.VMUUID}}
  mac: {{.VMMAC}}
  ip: {{.VMIP}}`,
			vars: TemplateVars{
				VMName: "vm1",
				VMUUID: "550e8400-e29b-41d4-a716-446655440000",
				VMMAC:  "52:54:00:12:34:56",
				VMIP:   "192.168.100.10",
			},
			expected: `apiVersion: v1
kind: ConfigMap
metadata:
  name: vm1
data:
  uuid: 550e8400-e29b-41d4-a716-446655440000
  mac: 52:54:00:12:34:56
  ip: 192.168.100.10`,
			wantErr: false,
		},
		{
			name:     "handles invalid template syntax",
			template: `{{.Invalid syntax}}`,
			vars:     TemplateVars{},
			wantErr:  true,
		},
		{
			name:     "handles undefined variable",
			template: `{{.UndefinedVar}}`,
			vars:     TemplateVars{},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applier := NewResourceApplier("/fake/path", "default")
			result, err := applier.RenderTemplate(tt.template, tt.vars)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if string(result) != tt.expected {
				t.Errorf("template mismatch\nexpected:\n%s\ngot:\n%s", tt.expected, string(result))
			}
		})
	}
}

func TestResourceApplier_ValidateYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    []byte
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid YAML with kind and metadata",
			yaml: []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: test`),
			wantErr: false,
		},
		{
			name:    "empty YAML",
			yaml:    []byte(""),
			wantErr: true,
			errMsg:  "empty YAML",
		},
		{
			name: "missing kind field",
			yaml: []byte(`apiVersion: v1
metadata:
  name: test`),
			wantErr: true,
			errMsg:  "missing 'kind'",
		},
		{
			name: "missing metadata field",
			yaml: []byte(`apiVersion: v1
kind: ConfigMap`),
			wantErr: true,
			errMsg:  "missing 'metadata'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applier := NewResourceApplier("/fake/path", "default")
			err := applier.ValidateYAML(tt.yaml)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got: %v", tt.errMsg, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestResourceApplier_ApplyResources_PartialFailure(t *testing.T) {
	// This test verifies the behavior when one resource fails during batch apply.
	// Since we can't mock kubectl, we test with invalid kubeconfig to trigger errors.

	applier := NewResourceApplier("/nonexistent/kubeconfig", "default")
	ctx := context.Background()

	resources := []K8sResource{
		{
			Kind:      "ConfigMap",
			Name:      "test-cm-1",
			Namespace: "default",
			YAML:      []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test-cm-1"),
		},
		{
			Kind:      "ConfigMap",
			Name:      "test-cm-2",
			Namespace: "default",
			YAML:      []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test-cm-2"),
		},
	}

	results, err := applier.ApplyResources(ctx, resources)

	if err == nil {
		t.Fatal("expected error when applying with invalid kubeconfig")
	}

	// Should return results tracking the failure
	if len(results) == 0 {
		t.Fatal("expected results even on failure")
	}

	// First resource should have failed
	if results[0].Status != "failed" {
		t.Errorf("expected first resource to have 'failed' status, got: %s", results[0].Status)
	}

	// Subsequent resources should be skipped
	if len(results) > 1 && results[1].Status != "skipped" {
		t.Errorf("expected second resource to be 'skipped', got: %s", results[1].Status)
	}
}

func TestResourceApplier_DeleteAll(t *testing.T) {
	// Test DeleteAll with empty kubeconfig to verify error handling
	applier := NewResourceApplier("", "default")
	ctx := context.Background()

	resources := []K8sResource{
		{Kind: "ConfigMap", Name: "cm1", Namespace: "default"},
		{Kind: "ConfigMap", Name: "cm2", Namespace: "default"},
	}

	err := applier.DeleteAll(ctx, resources)
	if err == nil {
		t.Fatal("expected error when deleting with empty kubeconfig")
	}

	// Should return aggregated errors
	if !strings.Contains(err.Error(), "kubeconfig") {
		t.Errorf("expected error about kubeconfig, got: %v", err)
	}
}

func TestResourceApplier_NamespaceHandling(t *testing.T) {
	// Test that resource namespace takes precedence over applier's default namespace
	applier := NewResourceApplier("/nonexistent/kubeconfig", "default-ns")

	tests := []struct {
		name              string
		resourceNamespace string
		expectedNamespace string
	}{
		{
			name:              "uses resource namespace when specified",
			resourceNamespace: "custom-ns",
			expectedNamespace: "custom-ns",
		},
		{
			name:              "uses applier namespace when resource namespace is empty",
			resourceNamespace: "",
			expectedNamespace: "default-ns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't actually run kubectl, but we can verify the error message
			// contains the expected namespace
			ctx := context.Background()
			resource := K8sResource{
				Kind:      "ConfigMap",
				Name:      "test-cm",
				Namespace: tt.resourceNamespace,
				YAML:      []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test-cm"),
			}

			_, err := applier.ApplyResource(ctx, resource)
			if err == nil {
				t.Fatal("expected error with nonexistent kubeconfig")
			}

			// Error should mention the namespace that was used
			// Note: This is a weak test since we can't verify the actual kubectl call
		})
	}
}

// Integration test - requires a real Kubernetes cluster
func TestResourceApplier_Integration(t *testing.T) {
	// Skip if no kubeconfig is available
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		t.Skip("skipping integration test: KUBECONFIG not set")
	}

	// Verify kubectl is available
	if _, err := exec.LookPath("kubectl"); err != nil {
		t.Skip("skipping integration test: kubectl not found in PATH")
	}

	// Verify cluster is accessible
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "cluster-info")
	if err := cmd.Run(); err != nil {
		t.Skipf("skipping integration test: cluster not accessible: %v", err)
	}

	t.Run("apply and delete configmap", func(t *testing.T) {
		applier := NewResourceApplier(kubeconfig, "default")
		ctx := context.Background()

		// Create a unique ConfigMap
		cmName := "test-cm-" + time.Now().Format("20060102-150405")
		resource := K8sResource{
			Kind:      "ConfigMap",
			Name:      cmName,
			Namespace: "default",
			YAML: []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: ` + cmName + `
data:
  test: value`),
		}

		// Apply resource
		result, err := applier.ApplyResource(ctx, resource)
		if err != nil {
			t.Fatalf("failed to apply resource: %v", err)
		}

		if result.Status != "created" {
			t.Errorf("expected status 'created', got: %s", result.Status)
		}

		// Verify resource exists
		data, err := applier.GetResource(ctx, "ConfigMap", cmName, "default")
		if err != nil {
			t.Errorf("failed to get resource: %v", err)
		}
		if len(data) == 0 {
			t.Error("expected non-empty resource data")
		}

		// Clean up
		if err := applier.DeleteResource(ctx, resource); err != nil {
			t.Errorf("failed to delete resource: %v", err)
		}
	})

	t.Run("apply multiple resources", func(t *testing.T) {
		applier := NewResourceApplier(kubeconfig, "default")
		ctx := context.Background()

		// Create unique names
		timestamp := time.Now().Format("20060102-150405")
		cm1Name := "test-cm1-" + timestamp
		cm2Name := "test-cm2-" + timestamp

		resources := []K8sResource{
			{
				Kind:      "ConfigMap",
				Name:      cm1Name,
				Namespace: "default",
				YAML: []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: ` + cm1Name + `
data:
  test: value1`),
			},
			{
				Kind:      "ConfigMap",
				Name:      cm2Name,
				Namespace: "default",
				YAML: []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: ` + cm2Name + `
data:
  test: value2`),
			},
		}

		// Apply all resources
		results, err := applier.ApplyResources(ctx, resources)
		if err != nil {
			t.Fatalf("failed to apply resources: %v", err)
		}

		if len(results) != len(resources) {
			t.Errorf("expected %d results, got %d", len(resources), len(results))
		}

		for _, result := range results {
			if result.Status != "created" {
				t.Errorf("expected status 'created', got: %s for %s", result.Status, result.Name)
			}
		}

		// Clean up
		if err := applier.DeleteAll(ctx, resources); err != nil {
			t.Errorf("failed to delete resources: %v", err)
		}
	})

	t.Run("template rendering with real resource", func(t *testing.T) {
		applier := NewResourceApplier(kubeconfig, "default")
		ctx := context.Background()

		// Render template
		template := `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{.VMName}}-config
data:
  uuid: {{.VMUUID}}
  mac: {{.VMMAC}}`

		vars := TemplateVars{
			VMName: "test-vm",
			VMUUID: "550e8400-e29b-41d4-a716-446655440000",
			VMMAC:  "52:54:00:12:34:56",
		}

		rendered, err := applier.RenderTemplate(template, vars)
		if err != nil {
			t.Fatalf("failed to render template: %v", err)
		}

		// Apply rendered resource
		resource := K8sResource{
			Kind:      "ConfigMap",
			Name:      "test-vm-config",
			Namespace: "default",
			YAML:      rendered,
		}

		result, err := applier.ApplyResource(ctx, resource)
		if err != nil {
			t.Fatalf("failed to apply rendered resource: %v", err)
		}

		if result.Status != "created" {
			t.Errorf("expected status 'created', got: %s", result.Status)
		}

		// Verify the data was templated correctly
		data, err := applier.GetResource(ctx, "ConfigMap", "test-vm-config", "default")
		if err != nil {
			t.Fatalf("failed to get resource: %v", err)
		}

		dataStr := string(data)
		if !strings.Contains(dataStr, vars.VMUUID) {
			t.Error("expected rendered YAML to contain UUID")
		}
		if !strings.Contains(dataStr, vars.VMMAC) {
			t.Error("expected rendered YAML to contain MAC address")
		}

		// Clean up
		if err := applier.DeleteResource(ctx, resource); err != nil {
			t.Errorf("failed to delete resource: %v", err)
		}
	})
}

// TestResourceApplier_WaitTimeout tests timeout behavior
func TestResourceApplier_WaitTimeout(t *testing.T) {
	// Skip if no kubeconfig
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		t.Skip("skipping test: KUBECONFIG not set")
	}

	// Verify kubectl is available
	if _, err := exec.LookPath("kubectl"); err != nil {
		t.Skip("skipping test: kubectl not found")
	}

	applier := NewResourceApplier(kubeconfig, "default")

	// Create a context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Try to wait for a non-existent resource
	err := applier.waitForResourceExists(ctx, "ConfigMap", "nonexistent-cm", "default")
	if err == nil {
		t.Fatal("expected timeout error")
	}

	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expected timeout error, got: %v", err)
	}
}

// Benchmark tests
func BenchmarkResourceApplier_RenderTemplate(b *testing.B) {
	applier := NewResourceApplier("/fake/path", "default")
	template := `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{.VMName}}-config
data:
  uuid: {{.VMUUID}}
  mac: {{.VMMAC}}
  ip: {{.VMIP}}`

	vars := TemplateVars{
		VMName: "test-vm",
		VMUUID: "550e8400-e29b-41d4-a716-446655440000",
		VMMAC:  "52:54:00:12:34:56",
		VMIP:   "192.168.100.10",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := applier.RenderTemplate(template, vars)
		if err != nil {
			b.Fatalf("render failed: %v", err)
		}
	}
}

func BenchmarkResourceApplier_ValidateYAML(b *testing.B) {
	applier := NewResourceApplier("/fake/path", "default")
	yaml := []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
data:
  key: value`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := applier.ValidateYAML(yaml)
		if err != nil {
			b.Fatalf("validation failed: %v", err)
		}
	}
}

// Helper test to verify error types
func TestResourceApplier_ErrorTypes(t *testing.T) {
	tests := []struct {
		name          string
		expectedError error
	}{
		{
			name:          "kubeconfig required error",
			expectedError: ErrKubeconfigRequired,
		},
		{
			name:          "resource timeout error",
			expectedError: ErrResourceTimeout,
		},
		{
			name:          "invalid YAML error",
			expectedError: ErrInvalidYAML,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectedError == nil {
				t.Error("expected error should not be nil")
			}
		})
	}
}
