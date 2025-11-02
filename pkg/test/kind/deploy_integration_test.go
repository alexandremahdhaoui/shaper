//go:build integration

package kind_test

import (
	"github.com/alexandremahdhaoui/shaper/pkg/test/kind"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Integration tests

func TestCreateNamespace_Integration(t *testing.T) {
	if !kind.IsKindInstalled() || !kind.IsKubectlInstalled() {
		t.Skip("KIND or kubectl not installed")
	}

	// Use the shared test cluster
	kubeconfigPath := getTestKubeconfig(t)

	// Test namespace creation
	namespace := "test-ns-" + uuid.NewString()[:8]
	err := kind.CreateNamespace(kubeconfigPath, namespace)
	require.NoError(t, err)

	// Verify namespace exists
	// We can do this by trying to create it again - should succeed (already exists)
	err = kind.CreateNamespace(kubeconfigPath, namespace)
	require.NoError(t, err)
}

func TestApplyManifest_Integration(t *testing.T) {
	if !kind.IsKindInstalled() || !kind.IsKubectlInstalled() {
		t.Skip("KIND or kubectl not installed")
	}

	// Use the shared test cluster
	kubeconfigPath := getTestKubeconfig(t)

	// Create a simple ConfigMap manifest
	manifestContent := `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-configmap
data:
  key: value
`
	manifestPath := filepath.Join(t.TempDir(), "configmap.yaml")
	err := os.WriteFile(manifestPath, []byte(manifestContent), 0644)
	require.NoError(t, err)

	// Apply manifest
	err = kind.ApplyManifest(kubeconfigPath, "default", manifestPath)
	require.NoError(t, err)

	// Clean up
	err = kind.DeleteManifest(kubeconfigPath, "default", manifestPath)
	require.NoError(t, err)
}

func TestCreateCRDs_Integration(t *testing.T) {
	if !kind.IsKindInstalled() || !kind.IsKubectlInstalled() {
		t.Skip("KIND or kubectl not installed")
	}

	// Use the shared test cluster
	kubeconfigPath := getTestKubeconfig(t)

	// Create a simple CRD manifest
	crdContent := `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: testresources.test.example.com
spec:
  group: test.example.com
  names:
    kind: TestResource
    listKind: TestResourceList
    plural: testresources
    singular: testresource
  scope: Namespaced
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            properties:
              field:
                type: string
`
	crdPath := filepath.Join(t.TempDir(), "crd.yaml")
	err := os.WriteFile(crdPath, []byte(crdContent), 0644)
	require.NoError(t, err)

	// Apply CRD
	err = kind.CreateCRDs(kubeconfigPath, []string{crdPath})
	require.NoError(t, err)

	// Clean up
	err = kind.DeleteManifest(kubeconfigPath, "", crdPath)
	require.NoError(t, err)
}

func TestGetPodStatus_Integration(t *testing.T) {
	if !kind.IsKindInstalled() || !kind.IsKubectlInstalled() {
		t.Skip("KIND or kubectl not installed")
	}

	// Use the shared test cluster
	kubeconfigPath := getTestKubeconfig(t)

	// Get pod status (might be empty, but should not error)
	status, err := kind.GetPodStatus(kubeconfigPath, "kube-system")
	require.NoError(t, err)
	require.NotEmpty(t, status)
	// Should contain header
	require.Contains(t, status, "NAME")
}

func TestDeployShaperToKIND_WithoutDeployment_Integration(t *testing.T) {
	if !kind.IsKindInstalled() || !kind.IsKubectlInstalled() {
		t.Skip("KIND or kubectl not installed")
	}

	// Use the shared test cluster
	kubeconfigPath := getTestKubeconfig(t)

	// Deploy without actual deployment (just namespace and CRDs)
	namespace := "test-shaper-" + uuid.NewString()[:8]
	deployConfig := kind.DeployConfig{
		Kubeconfig:  kubeconfigPath,
		Namespace:   namespace,
		WaitTimeout: 30 * time.Second,
	}

	err := kind.DeployShaperToKIND(deployConfig)
	require.NoError(t, err)
}

func TestCreateTestProfile_Integration(t *testing.T) {
	if !kind.IsKindInstalled() || !kind.IsKubectlInstalled() {
		t.Skip("KIND or kubectl not installed")
	}

	// Use the shared test cluster
	kubeconfigPath := getTestKubeconfig(t)

	// Note: The Shaper CRDs should already be installed by `make test-setup`
	// Create a test profile with a unique name
	profileName := "test-profile-" + uuid.NewString()[:8]
	profileYAML := []byte(`apiVersion: shaper.amahdha.com/v1alpha1
kind: Profile
metadata:
  name: ` + profileName + `
spec:
  ipxeTemplate: |
    #!ipxe
    echo Test profile
`)

	err := kind.CreateTestProfile(kubeconfigPath, "default", profileName, profileYAML)
	require.NoError(t, err)
}

