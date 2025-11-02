//go:build integration

package network

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Integration tests for bridge management

func TestCreateBridge_Integration(t *testing.T) {
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

func TestBridgeExists_Integration(t *testing.T) {
	// Linux interface names limited to 15 chars
	bridgeName := "br" + uuid.NewString()[:6]
	config := BridgeConfig{
		Name: bridgeName,
		CIDR: "192.168.203.1/24",
	}

	// Before creation - should not exist
	exists, err := BridgeExists(bridgeName)
	require.NoError(t, err)
	require.False(t, exists)

	// Create bridge
	err = CreateBridge(config)
	require.NoError(t, err)
	defer DeleteBridge(bridgeName)

	// After creation - should exist
	exists, err = BridgeExists(bridgeName)
	require.NoError(t, err)
	require.True(t, exists)
}
