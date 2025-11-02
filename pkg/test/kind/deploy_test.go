package kind_test

import (
	"testing"

	"github.com/alexandremahdhaoui/shaper/pkg/test/kind"
	"github.com/stretchr/testify/require"
)

func TestDeployConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  kind.DeployConfig
		wantErr error
	}{
		{
			name: "valid config",
			config: kind.DeployConfig{
				Kubeconfig: "/tmp/kubeconfig",
				Namespace:  "default",
			},
			wantErr: nil,
		},
		{
			name: "missing kubeconfig",
			config: kind.DeployConfig{
				Kubeconfig: "",
				Namespace:  "default",
			},
			wantErr: kind.ErrKubeconfigRequired,
		},
		{
			name: "missing namespace",
			config: kind.DeployConfig{
				Kubeconfig: "/tmp/kubeconfig",
				Namespace:  "",
			},
			wantErr: kind.ErrNamespaceRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr != nil {
				err := kind.DeployShaperToKIND(tt.config)
				require.ErrorIs(t, err, tt.wantErr)
			}
		})
	}
}

func TestIsKubectlInstalled(t *testing.T) {
	// This test just verifies the function works
	// Result depends on environment
	installed := kind.IsKubectlInstalled()
	t.Logf("kubectl installed: %v", installed)
}

func TestApplyManifest_FileNotFound(t *testing.T) {
	err := kind.ApplyManifest("/tmp/kubeconfig", "default", "/nonexistent/file.yaml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not exist")
}
