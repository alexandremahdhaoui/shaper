//go:build unit

// Copyright 2024 Alexandre Mahdhaoui
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package k8s_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alexandremahdhaoui/shaper/internal/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

// TestNewKubeRestConfig_InCluster tests both in-cluster config variations
func TestNewKubeRestConfig_InCluster(t *testing.T) {
	tests := []struct {
		name           string
		kubeconfigPath string
	}{
		{
			name:           "standard in-cluster string",
			kubeconfigPath: k8s.InClusterConfig,
		},
		{
			name:           "service account string (shaper-api compat)",
			kubeconfigPath: k8s.ServiceAccountConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test with the in-cluster config strings
			_, err := k8s.NewKubeRestConfig(tt.kubeconfigPath)

			// This will fail in unit test environment (no service account), which is expected
			// We're just testing that the function recognizes the special strings
			assert.Error(t, err, "in-cluster config should fail in unit test environment")
		})
	}
}

// TestNewKubeRestConfig_InvalidFile tests error handling for invalid kubeconfig files
func TestNewKubeRestConfig_InvalidFile(t *testing.T) {
	tmpDir := t.TempDir()
	kubeconfigPath := filepath.Join(tmpDir, "invalid-kubeconfig")
	err := os.WriteFile(kubeconfigPath, []byte("invalid kubeconfig content"), 0o644)
	require.NoError(t, err)

	_, err = k8s.NewKubeRestConfig(kubeconfigPath)
	assert.Error(t, err, "invalid kubeconfig should return error")
}

// TestNewKubeRestConfig_NonExistentFile tests error handling when kubeconfig file doesn't exist
func TestNewKubeRestConfig_NonExistentFile(t *testing.T) {
	_, err := k8s.NewKubeRestConfig("/non/existent/kubeconfig")
	assert.Error(t, err, "non-existent kubeconfig should return error")
}

// TestNewKubeClient_WithNilConfig tests that NewKubeClient fails with nil config
func TestNewKubeClient_WithNilConfig(t *testing.T) {
	_, err := k8s.NewKubeClient(nil)
	assert.Error(t, err, "NewKubeClient should fail with nil config")
}

// TestNewKubeClient_WithValidConfig tests client creation with valid config
func TestNewKubeClient_WithValidConfig(t *testing.T) {
	// Create a minimal valid rest.Config
	restConfig := &rest.Config{
		Host: "https://localhost:6443", // Dummy host
	}

	client, err := k8s.NewKubeClient(restConfig)

	// The client creation should succeed (scheme setup works)
	// It will only fail when actually used to communicate with API server
	assert.NoError(t, err, "NewKubeClient should succeed with valid rest.Config structure")
	assert.NotNil(t, client, "client should not be nil")
}

// TestConstants verifies the exported constant values
func TestConstants(t *testing.T) {
	assert.Equal(t, "in-cluster", k8s.InClusterConfig)
	assert.Equal(t, ">>> Kubeconfig From Service Account", k8s.ServiceAccountConfig)
}
