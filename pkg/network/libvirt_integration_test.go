//go:build integration

package network_test

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"github.com/alexandremahdhaoui/shaper/pkg/network"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"libvirt.org/go/libvirt"
)

// generateUniqueIP generates a unique IP in 192.168.{octet}.1 range
// to avoid conflicts between parallel tests
func generateUniqueIP() string {
	// Use range 160-250 to avoid conflicts with default networks
	octet := 160 + rand.Intn(90)
	return fmt.Sprintf("192.168.%d.1", octet)
}

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
		Name:      networkName,
		Mode:      "nat",
		IPAddress: generateUniqueIP(), // Use unique IP to avoid conflicts
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
		Name:      networkName,
		Mode:      "isolated",
		IPAddress: generateUniqueIP(), // Use unique IP to avoid conflicts
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
		Name:      networkName,
		Mode:      "isolated",
		IPAddress: generateUniqueIP(), // Use unique IP to avoid conflicts
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
		Name:      networkName,
		Mode:      "isolated",
		IPAddress: generateUniqueIP(), // Use unique IP to avoid conflicts
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
