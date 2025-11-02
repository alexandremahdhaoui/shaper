//go:build unit

package kind_test

import (
	"testing"

	"github.com/alexandremahdhaoui/shaper/pkg/test/kind"
	"github.com/stretchr/testify/require"
)

func TestClusterConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  kind.ClusterConfig
		wantErr error
	}{
		{
			name: "valid config",
			config: kind.ClusterConfig{
				Name: "test-cluster",
			},
			wantErr: nil,
		},
		{
			name: "empty name",
			config: kind.ClusterConfig{
				Name: "",
			},
			wantErr: kind.ErrClusterNameRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.config.Name == "" {
				err := kind.CreateCluster(tt.config)
				require.ErrorIs(t, err, tt.wantErr)
			}
		})
	}
}
