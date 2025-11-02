//go:build integration

package network

import (
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

	config := LibvirtNetworkConfig{
		Name:       networkName,
		BridgeName: bridgeName,
		Mode:       "bridge",
	}

	err = CreateLibvirtNetwork(conn, config)
	require.NoError(t, err)
	defer DeleteLibvirtNetwork(conn, networkName)

	// Verify network exists
	exists, err := NetworkExists(conn, networkName)
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

	config := LibvirtNetworkConfig{
		Name: networkName,
		Mode: "nat",
	}

	err = CreateLibvirtNetwork(conn, config)
	require.NoError(t, err)
	defer DeleteLibvirtNetwork(conn, networkName)

	// Verify network exists
	exists, err := NetworkExists(conn, networkName)
	require.NoError(t, err)
	require.True(t, exists)
}

func TestCreateLibvirtNetwork_Idempotent_Integration(t *testing.T) {
	conn, err := libvirt.NewConnect("qemu:///system")
	require.NoError(t, err)
	defer conn.Close()

	networkName := "net" + uuid.NewString()[:8]

	config := LibvirtNetworkConfig{
		Name: networkName,
		Mode: "isolated",
	}

	// Create first time
	err = CreateLibvirtNetwork(conn, config)
	require.NoError(t, err)
	defer DeleteLibvirtNetwork(conn, networkName)

	// Create second time - should not error
	err = CreateLibvirtNetwork(conn, config)
	require.NoError(t, err)

	// Verify network still exists
	exists, err := NetworkExists(conn, networkName)
	require.NoError(t, err)
	require.True(t, exists)
}

func TestDeleteLibvirtNetwork_Integration(t *testing.T) {
	conn, err := libvirt.NewConnect("qemu:///system")
	require.NoError(t, err)
	defer conn.Close()

	networkName := "net" + uuid.NewString()[:8]

	config := LibvirtNetworkConfig{
		Name: networkName,
		Mode: "isolated",
	}

	// Create network
	err = CreateLibvirtNetwork(conn, config)
	require.NoError(t, err)

	// Verify it exists
	exists, err := NetworkExists(conn, networkName)
	require.NoError(t, err)
	require.True(t, exists)

	// Delete network
	err = DeleteLibvirtNetwork(conn, networkName)
	require.NoError(t, err)

	// Verify it's gone
	exists, err = NetworkExists(conn, networkName)
	require.NoError(t, err)
	require.False(t, exists)
}

func TestDeleteLibvirtNetwork_Idempotent_Integration(t *testing.T) {
	conn, err := libvirt.NewConnect("qemu:///system")
	require.NoError(t, err)
	defer conn.Close()

	networkName := "net" + uuid.NewString()[:8]

	config := LibvirtNetworkConfig{
		Name: networkName,
		Mode: "isolated",
	}

	// Create and delete network
	err = CreateLibvirtNetwork(conn, config)
	require.NoError(t, err)

	err = DeleteLibvirtNetwork(conn, networkName)
	require.NoError(t, err)

	// Delete again - should not error
	err = DeleteLibvirtNetwork(conn, networkName)
	require.NoError(t, err)
}

func TestNetworkExists_NonExistent_Integration(t *testing.T) {
	conn, err := libvirt.NewConnect("qemu:///system")
	require.NoError(t, err)
	defer conn.Close()

	// Check for network that doesn't exist
	exists, err := NetworkExists(conn, "nonexistent-net-"+uuid.NewString())
	require.NoError(t, err)
	require.False(t, exists)
}
