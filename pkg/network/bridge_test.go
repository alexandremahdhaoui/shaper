package network_test

import (
	"testing"

	"github.com/alexandremahdhaoui/shaper/pkg/network"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestBridgeConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  network.BridgeConfig
		wantErr error
	}{
		{
			name: "valid config",
			config: network.BridgeConfig{
				Name: "br-test",
				CIDR: "192.168.100.1/24",
			},
			wantErr: nil,
		},
		{
			name: "empty name",
			config: network.BridgeConfig{
				Name: "",
				CIDR: "192.168.100.1/24",
			},
			wantErr: network.ErrBridgeNameRequired,
		},
		{
			name: "empty CIDR",
			config: network.BridgeConfig{
				Name: "br-test",
				CIDR: "",
			},
			wantErr: network.ErrCIDRRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We don't actually create the bridge in unit tests
			// Just verify validation logic
			if tt.config.Name == "" {
				require.ErrorIs(t, network.CreateBridge(tt.config), tt.wantErr)
			} else if tt.config.CIDR == "" {
				require.ErrorIs(t, network.CreateBridge(tt.config), tt.wantErr)
			}
		})
	}
}

func TestBridgeExists_InvalidName(t *testing.T) {
	exists, err := network.BridgeExists("")
	require.ErrorIs(t, err, network.ErrBridgeNameRequired)
	require.False(t, exists)
}

func TestBridgeExists_NonExistent(t *testing.T) {
	// Test with a bridge that definitely doesn't exist
	bridgeName := "nonexistent-bridge-" + uuid.NewString()[:8]
	exists, err := network.BridgeExists(bridgeName)
	require.NoError(t, err)
	require.False(t, exists)
}

func TestDeleteBridge_InvalidName(t *testing.T) {
	err := network.DeleteBridge("")
	require.ErrorIs(t, err, network.ErrBridgeNameRequired)
}

func TestDeleteBridge_NonExistent(t *testing.T) {
	// Deleting non-existent bridge should not error (idempotent)
	bridgeName := "nonexistent-bridge-" + uuid.NewString()[:8]
	err := network.DeleteBridge(bridgeName)
	require.NoError(t, err)
}
