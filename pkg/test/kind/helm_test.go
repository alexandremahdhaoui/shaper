//go:build unit

package kind_test

import (
	"testing"

	"github.com/alexandremahdhaoui/shaper/pkg/test/kind"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsHelmInstalled_Unit(t *testing.T) {
	// This test just checks that the function returns a boolean
	// It doesn't verify helm is actually installed
	result := kind.IsHelmInstalled()
	assert.IsType(t, false, result)
}

func TestHelmInstall_ValidationErrors_Unit(t *testing.T) {
	tests := []struct {
		name        string
		config      kind.HelmConfig
		expectedErr error
	}{
		{
			name: "missing kubeconfig",
			config: kind.HelmConfig{
				Namespace:   "test",
				ReleaseName: "test-release",
				ChartPath:   "/path/to/chart",
			},
			expectedErr: kind.ErrKubeconfigRequired,
		},
		{
			name: "missing namespace",
			config: kind.HelmConfig{
				Kubeconfig:  "/tmp/kubeconfig",
				ReleaseName: "test-release",
				ChartPath:   "/path/to/chart",
			},
			expectedErr: kind.ErrNamespaceRequired,
		},
		{
			name: "missing release name",
			config: kind.HelmConfig{
				Kubeconfig: "/tmp/kubeconfig",
				Namespace:  "test",
				ChartPath:  "/path/to/chart",
			},
			expectedErr: kind.ErrRelease,
		},
		{
			name: "missing chart path",
			config: kind.HelmConfig{
				Kubeconfig:  "/tmp/kubeconfig",
				Namespace:   "test",
				ReleaseName: "test-release",
			},
			expectedErr: kind.ErrChartPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := kind.HelmInstall(tt.config)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.expectedErr)
		})
	}
}

func TestHelmUninstall_ValidationErrors_Unit(t *testing.T) {
	tests := []struct {
		name        string
		kubeconfig  string
		namespace   string
		releaseName string
		expectedErr error
	}{
		{
			name:        "missing kubeconfig",
			kubeconfig:  "",
			namespace:   "test",
			releaseName: "test-release",
			expectedErr: kind.ErrKubeconfigRequired,
		},
		{
			name:        "missing namespace",
			kubeconfig:  "/tmp/kubeconfig",
			namespace:   "",
			releaseName: "test-release",
			expectedErr: kind.ErrNamespaceRequired,
		},
		{
			name:        "missing release name",
			kubeconfig:  "/tmp/kubeconfig",
			namespace:   "test",
			releaseName: "",
			expectedErr: kind.ErrRelease,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := kind.HelmUninstall(tt.kubeconfig, tt.namespace, tt.releaseName)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.expectedErr)
		})
	}
}

func TestPortForwardService_ValidationErrors_Unit(t *testing.T) {
	tests := []struct {
		name        string
		kubeconfig  string
		namespace   string
		serviceName string
		localPort   string
		remotePort  string
		expectedErr error
	}{
		{
			name:        "missing kubeconfig",
			kubeconfig:  "",
			namespace:   "test",
			serviceName: "test-service",
			localPort:   "8080",
			remotePort:  "80",
			expectedErr: kind.ErrKubeconfigRequired,
		},
		{
			name:        "missing namespace",
			kubeconfig:  "/tmp/kubeconfig",
			namespace:   "",
			serviceName: "test-service",
			localPort:   "8080",
			remotePort:  "80",
			expectedErr: kind.ErrNamespaceRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup, err := kind.PortForwardService(tt.kubeconfig, tt.namespace, tt.serviceName, tt.localPort, tt.remotePort)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.expectedErr)
			assert.Nil(t, cleanup)
		})
	}
}
