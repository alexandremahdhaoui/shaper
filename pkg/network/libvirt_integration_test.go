//go:build integration

package network_test

import (
	"context"
	"testing"

	"github.com/alexandremahdhaoui/shaper/pkg/network"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"libvirt.org/go/libvirt"
)

// Integration tests for libvirt network management

func TestLibvirtNetworkManager_Create_Bridge_Integration(t *testing.T) {
	conn, err := libvirt.NewConnect("qemu:///system")
	require.NoError(t, err)
	defer conn.Close()

	mgr := network.NewLibvirtNetworkManager(conn)
	ctx := context.Background()

	networkName := "net" + uuid.NewString()[:8]

	// For bridge mode, we need an existing bridge
	// Use the default libvirt bridge if it exists, otherwise skip
	bridgeName := "virbr0"

	config := network.LibvirtNetworkConfig{
		Name:       networkName,
		BridgeName: bridgeName,
		Mode:       "bridge",
	}

	err = mgr.Create(ctx, config)
	require.NoError(t, err)
	defer mgr.Delete(ctx, networkName)

	// Verify network exists using Get
	info, err := mgr.Get(ctx, networkName)
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Equal(t, networkName, info.Name)
	require.True(t, info.IsActive)
}

func TestLibvirtNetworkManager_Create_NAT_Integration(t *testing.T) {
	conn, err := libvirt.NewConnect("qemu:///system")
	require.NoError(t, err)
	defer conn.Close()

	mgr := network.NewLibvirtNetworkManager(conn)
	ctx := context.Background()

	networkName := "net" + uuid.NewString()[:8]

	config := network.LibvirtNetworkConfig{
		Name: networkName,
		Mode: "nat",
	}

	err = mgr.Create(ctx, config)
	require.NoError(t, err)
	defer mgr.Delete(ctx, networkName)

	// Verify network exists using Get
	info, err := mgr.Get(ctx, networkName)
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Equal(t, networkName, info.Name)
}

func TestLibvirtNetworkManager_Create_Idempotent_Integration(t *testing.T) {
	conn, err := libvirt.NewConnect("qemu:///system")
	require.NoError(t, err)
	defer conn.Close()

	mgr := network.NewLibvirtNetworkManager(conn)
	ctx := context.Background()

	networkName := "net" + uuid.NewString()[:8]

	config := network.LibvirtNetworkConfig{
		Name: networkName,
		Mode: "isolated",
	}

	// Create first time
	err = mgr.Create(ctx, config)
	require.NoError(t, err)
	defer mgr.Delete(ctx, networkName)

	// Create second time - should not error
	err = mgr.Create(ctx, config)
	require.NoError(t, err)

	// Verify network still exists
	info, err := mgr.Get(ctx, networkName)
	require.NoError(t, err)
	require.NotNil(t, info)
}

func TestLibvirtNetworkManager_Delete_Integration(t *testing.T) {
	conn, err := libvirt.NewConnect("qemu:///system")
	require.NoError(t, err)
	defer conn.Close()

	mgr := network.NewLibvirtNetworkManager(conn)
	ctx := context.Background()

	networkName := "net" + uuid.NewString()[:8]

	config := network.LibvirtNetworkConfig{
		Name: networkName,
		Mode: "isolated",
	}

	// Create network
	err = mgr.Create(ctx, config)
	require.NoError(t, err)

	// Verify it exists
	info, err := mgr.Get(ctx, networkName)
	require.NoError(t, err)
	require.NotNil(t, info)

	// Delete network
	err = mgr.Delete(ctx, networkName)
	require.NoError(t, err)

	// Verify it's gone (Get should return error)
	_, err = mgr.Get(ctx, networkName)
	require.Error(t, err)
}

func TestLibvirtNetworkManager_Delete_Idempotent_Integration(t *testing.T) {
	conn, err := libvirt.NewConnect("qemu:///system")
	require.NoError(t, err)
	defer conn.Close()

	mgr := network.NewLibvirtNetworkManager(conn)
	ctx := context.Background()

	networkName := "net" + uuid.NewString()[:8]

	config := network.LibvirtNetworkConfig{
		Name: networkName,
		Mode: "isolated",
	}

	// Create and delete network
	err = mgr.Create(ctx, config)
	require.NoError(t, err)

	err = mgr.Delete(ctx, networkName)
	require.NoError(t, err)

	// Delete again - should not error
	err = mgr.Delete(ctx, networkName)
	require.NoError(t, err)
}

func TestLibvirtNetworkManager_Get_NonExistent_Integration(t *testing.T) {
	conn, err := libvirt.NewConnect("qemu:///system")
	require.NoError(t, err)
	defer conn.Close()

	mgr := network.NewLibvirtNetworkManager(conn)
	ctx := context.Background()

	// Check for network that doesn't exist
	_, err = mgr.Get(ctx, "nonexistent-net-"+uuid.NewString())
	require.Error(t, err)
}
