package network

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestBridgeConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  BridgeConfig
		wantErr error
	}{
		{
			name: "valid config",
			config: BridgeConfig{
				Name: "br-test",
				CIDR: "192.168.100.1/24",
			},
			wantErr: nil,
		},
		{
			name: "empty name",
			config: BridgeConfig{
				Name: "",
				CIDR: "192.168.100.1/24",
			},
			wantErr: errBridgeNameRequired,
		},
		{
			name: "empty CIDR",
			config: BridgeConfig{
				Name: "br-test",
				CIDR: "",
			},
			wantErr: errCIDRRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We don't actually create the bridge in unit tests
			// Just verify validation logic
			if tt.config.Name == "" {
				require.ErrorIs(t, CreateBridge(tt.config), tt.wantErr)
			} else if tt.config.CIDR == "" {
				require.ErrorIs(t, CreateBridge(tt.config), tt.wantErr)
			}
		})
	}
}

func TestBridgeExists_InvalidName(t *testing.T) {
	exists, err := BridgeExists("")
	require.ErrorIs(t, err, errBridgeNameRequired)
	require.False(t, exists)
}

func TestBridgeExists_NonExistent(t *testing.T) {
	// Test with a bridge that definitely doesn't exist
	bridgeName := "nonexistent-bridge-" + uuid.NewString()[:8]
	exists, err := BridgeExists(bridgeName)
	require.NoError(t, err)
	require.False(t, exists)
}

func TestDeleteBridge_InvalidName(t *testing.T) {
	err := DeleteBridge("")
	require.ErrorIs(t, err, errBridgeNameRequired)
}

func TestDeleteBridge_NonExistent(t *testing.T) {
	// Deleting non-existent bridge should not error (idempotent)
	bridgeName := "nonexistent-bridge-" + uuid.NewString()[:8]
	err := DeleteBridge(bridgeName)
	require.NoError(t, err)
}



	// Linux interface names limited to 15 chars
	bridgeName := "br" + uuid.NewString()[:6]
	config := BridgeConfig{
		Name: bridgeName,
		CIDR: "192.168.200.1/24",
	}

	// Create bridge
	err := CreateBridge(config)
	require.NoError(t, err)
	defer DeleteBridge(bridgeName)

	// Verify bridge exists
	exists, err := BridgeExists(bridgeName)
	require.NoError(t, err)
	require.True(t, exists)
}

func TestCreateBridge_Idempotent_Integration(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root privileges")
	}

	// Linux interface names limited to 15 chars
	bridgeName := "br" + uuid.NewString()[:6]
	config := BridgeConfig{
		Name: bridgeName,
		CIDR: "192.168.201.1/24",
	}

	// Create bridge first time
	err := CreateBridge(config)
	require.NoError(t, err)
	defer DeleteBridge(bridgeName)

	// Create bridge second time - should not error
	err = CreateBridge(config)
	require.NoError(t, err)

	// Verify bridge still exists
	exists, err := BridgeExists(bridgeName)
	require.NoError(t, err)
	require.True(t, exists)
}

func TestDeleteBridge_Integration(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root privileges")
	}

	// Linux interface names limited to 15 chars
	bridgeName := "br" + uuid.NewString()[:6]
	config := BridgeConfig{
		Name: bridgeName,
		CIDR: "192.168.202.1/24",
	}

	// Create bridge
	err := CreateBridge(config)
	require.NoError(t, err)

	// Verify it exists
	exists, err := BridgeExists(bridgeName)
	require.NoError(t, err)
	require.True(t, exists)

	// Delete bridge
	err = DeleteBridge(bridgeName)
	require.NoError(t, err)

	// Verify it's gone
	exists, err = BridgeExists(bridgeName)
	require.NoError(t, err)
	require.False(t, exists)
}

func TestDeleteBridge_Idempotent_Integration(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root privileges")
	}

	// Linux interface names limited to 15 chars
	bridgeName := "br" + uuid.NewString()[:6]
	config := BridgeConfig{
		Name: bridgeName,
		CIDR: "192.168.203.1/24",
	}

	// Create and delete bridge
	err := CreateBridge(config)
	require.NoError(t, err)

	err = DeleteBridge(bridgeName)
	require.NoError(t, err)

	// Delete again - should not error
	err = DeleteBridge(bridgeName)
	require.NoError(t, err)
}
