//go:build integration

package network_test

import (
	"github.com/alexandremahdhaoui/shaper/pkg/network"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Integration tests for bridge management

func TestCreateBridge_Integration(t *testing.T) {
	// Linux interface names limited to 15 chars
	bridgeName := "br" + uuid.NewString()[:6]
	config := network.BridgeConfig{
		Name: bridgeName,
		CIDR: "192.168.200.1/24",
	}

	// Create bridge
	err := network.CreateBridge(config)
	require.NoError(t, err)
	defer network.DeleteBridge(bridgeName)

	// Verify bridge exists
	exists, err := network.BridgeExists(bridgeName)
	require.NoError(t, err)
	require.True(t, exists)
}

func TestCreateBridge_Idempotent_Integration(t *testing.T) {
	// Linux interface names limited to 15 chars
	bridgeName := "br" + uuid.NewString()[:6]
	config := network.BridgeConfig{
		Name: bridgeName,
		CIDR: "192.168.201.1/24",
	}

	// Create bridge first time
	err := network.CreateBridge(config)
	require.NoError(t, err)
	defer network.DeleteBridge(bridgeName)

	// Create bridge second time - should not error
	err = network.CreateBridge(config)
	require.NoError(t, err)

	// Verify bridge still exists
	exists, err := network.BridgeExists(bridgeName)
	require.NoError(t, err)
	require.True(t, exists)
}

func TestDeleteBridge_Integration(t *testing.T) {
	// Linux interface names limited to 15 chars
	bridgeName := "br" + uuid.NewString()[:6]
	config := network.BridgeConfig{
		Name: bridgeName,
		CIDR: "192.168.202.1/24",
	}

	// Create bridge
	err := network.CreateBridge(config)
	require.NoError(t, err)

	// Verify it exists
	exists, err := network.BridgeExists(bridgeName)
	require.NoError(t, err)
	require.True(t, exists)

	// Delete bridge
	err = network.DeleteBridge(bridgeName)
	require.NoError(t, err)

	// Verify it's gone
	exists, err = network.BridgeExists(bridgeName)
	require.NoError(t, err)
	require.False(t, exists)
}

func TestBridgeExists_Integration(t *testing.T) {
	// Linux interface names limited to 15 chars
	bridgeName := "br" + uuid.NewString()[:6]
	config := network.BridgeConfig{
		Name: bridgeName,
		CIDR: "192.168.203.1/24",
	}

	// Before creation - should not exist
	exists, err := network.BridgeExists(bridgeName)
	require.NoError(t, err)
	require.False(t, exists)

	// Create bridge
	err = network.CreateBridge(config)
	require.NoError(t, err)
	defer network.DeleteBridge(bridgeName)

	// After creation - should exist
	exists, err = network.BridgeExists(bridgeName)
	require.NoError(t, err)
	require.True(t, exists)
}
