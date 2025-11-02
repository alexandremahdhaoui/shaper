//go:build integration

package network_test

import (
	"github.com/alexandremahdhaoui/shaper/pkg/network"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"libvirt.org/go/libvirt"
)

// Integration tests for libvirt network management

func TestCreateLibvirtNetwork_Bridge_Integration(t *testing.T) {
	conn, err := libvirt.NewConnect("qemu:///system")
	require.NoError(t, err)
	defer conn.Close()

	networkName := "net" + uuid.NewString()[:8]

	// For bridge mode, we need an existing bridge
	// Use the default libvirt bridge if it exists, otherwise skip
	bridgeName := "virbr0"

	config := network.LibvirtNetworkConfig{
		Name:       networkName,
		BridgeName: bridgeName,
		Mode:       "bridge",
	}

	err = network.CreateLibvirtNetwork(conn, config)
	require.NoError(t, err)
	defer network.DeleteLibvirtNetwork(conn, networkName)

	// Verify network exists
	exists, err := network.NetworkExists(conn, networkName)
	require.NoError(t, err)
	require.True(t, exists)

	// Verify network is active
	network, err := conn.LookupNetworkByName(networkName)
	require.NoError(t, err)
	defer network.Free()

	active, err := network.IsActive()
	require.NoError(t, err)
	require.True(t, active)
}

func TestCreateLibvirtNetwork_NAT_Integration(t *testing.T) {
	conn, err := libvirt.NewConnect("qemu:///system")
	require.NoError(t, err)
	defer conn.Close()

	networkName := "net" + uuid.NewString()[:8]

	config := network.LibvirtNetworkConfig{
		Name: networkName,
		Mode: "nat",
	}

	err = network.CreateLibvirtNetwork(conn, config)
	require.NoError(t, err)
	defer network.DeleteLibvirtNetwork(conn, networkName)

	// Verify network exists
	exists, err := network.NetworkExists(conn, networkName)
	require.NoError(t, err)
	require.True(t, exists)
}

func TestCreateLibvirtNetwork_Idempotent_Integration(t *testing.T) {
	conn, err := libvirt.NewConnect("qemu:///system")
	require.NoError(t, err)
	defer conn.Close()

	networkName := "net" + uuid.NewString()[:8]

	config := network.LibvirtNetworkConfig{
		Name: networkName,
		Mode: "isolated",
	}

	// Create first time
	err = network.CreateLibvirtNetwork(conn, config)
	require.NoError(t, err)
	defer network.DeleteLibvirtNetwork(conn, networkName)

	// Create second time - should not error
	err = network.CreateLibvirtNetwork(conn, config)
	require.NoError(t, err)

	// Verify network still exists
	exists, err := network.NetworkExists(conn, networkName)
	require.NoError(t, err)
	require.True(t, exists)
}

func TestDeleteLibvirtNetwork_Integration(t *testing.T) {
	conn, err := libvirt.NewConnect("qemu:///system")
	require.NoError(t, err)
	defer conn.Close()

	networkName := "net" + uuid.NewString()[:8]

	config := network.LibvirtNetworkConfig{
		Name: networkName,
		Mode: "isolated",
	}

	// Create network
	err = network.CreateLibvirtNetwork(conn, config)
	require.NoError(t, err)

	// Verify it exists
	exists, err := network.NetworkExists(conn, networkName)
	require.NoError(t, err)
	require.True(t, exists)

	// Delete network
	err = network.DeleteLibvirtNetwork(conn, networkName)
	require.NoError(t, err)

	// Verify it's gone
	exists, err = network.NetworkExists(conn, networkName)
	require.NoError(t, err)
	require.False(t, exists)
}

func TestDeleteLibvirtNetwork_Idempotent_Integration(t *testing.T) {
	conn, err := libvirt.NewConnect("qemu:///system")
	require.NoError(t, err)
	defer conn.Close()

	networkName := "net" + uuid.NewString()[:8]

	config := network.LibvirtNetworkConfig{
		Name: networkName,
		Mode: "isolated",
	}

	// Create and delete network
	err = network.CreateLibvirtNetwork(conn, config)
	require.NoError(t, err)

	err = network.DeleteLibvirtNetwork(conn, networkName)
	require.NoError(t, err)

	// Delete again - should not error
	err = network.DeleteLibvirtNetwork(conn, networkName)
	require.NoError(t, err)
}

func TestNetworkExists_NonExistent_Integration(t *testing.T) {
	conn, err := libvirt.NewConnect("qemu:///system")
	require.NoError(t, err)
	defer conn.Close()

	// Check for network that doesn't exist
	exists, err := network.NetworkExists(conn, "nonexistent-net-"+uuid.NewString())
	require.NoError(t, err)
	require.False(t, exists)
}
