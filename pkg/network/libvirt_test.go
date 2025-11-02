package network

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"libvirt.org/go/libvirt"
)

func TestLibvirtNetworkConfig_Validation(t *testing.T) {
	// These tests verify validation without requiring libvirt
	t.Run("nil connection", func(t *testing.T) {
		config := LibvirtNetworkConfig{
			Name: "test",
			Mode: "bridge",
		}
		err := CreateLibvirtNetwork(nil, config)
		require.ErrorIs(t, err, errConnNil)
	})

	t.Run("empty name", func(t *testing.T) {
		// We can't create a real connection for unit test
		// Just verify the error types are correct
		err := DeleteLibvirtNetwork(nil, "")
		require.ErrorIs(t, err, errNetworkNameRequired)

		exists, err := NetworkExists(nil, "")
		require.ErrorIs(t, err, errNetworkNameRequired)
		require.False(t, exists)
	})
}

func TestGenerateNetworkXML(t *testing.T) {
	tests := []struct {
		name       string
		config     LibvirtNetworkConfig
		shouldFail bool
		contains   []string
	}{
		{
			name: "bridge mode",
			config: LibvirtNetworkConfig{
				Name:       "test-net",
				BridgeName: "br-test",
				Mode:       "bridge",
			},
			shouldFail: false,
			contains: []string{
				"<name>test-net</name>",
				"<forward mode=\"bridge\"",
				"<bridge name=\"br-test\"",
			},
		},
		{
			name: "nat mode",
			config: LibvirtNetworkConfig{
				Name: "test-nat",
				Mode: "nat",
			},
			shouldFail: false,
			contains: []string{
				"<name>test-nat</name>",
				"<forward mode=\"nat\"",
			},
		},
		{
			name: "isolated mode",
			config: LibvirtNetworkConfig{
				Name: "test-isolated",
				Mode: "isolated",
			},
			shouldFail: false,
			contains: []string{
				"<name>test-isolated</name>",
			},
		},
		{
			name: "bridge mode without bridge name",
			config: LibvirtNetworkConfig{
				Name: "test-fail",
				Mode: "bridge",
			},
			shouldFail: true,
		},
		{
			name: "unsupported mode",
			config: LibvirtNetworkConfig{
				Name: "test-fail",
				Mode: "invalid",
			},
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xml, err := generateNetworkXML(tt.config)
			if tt.shouldFail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			for _, substr := range tt.contains {
				require.Contains(t, xml, substr)
			}
		})
	}
}



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
	if os.Getenv("CI") == "true" && os.Getenv("LIBVIRT_TEST") != "true" {
		t.Skip("skipping libvirt integration test in CI")
	}

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
	if os.Getenv("CI") == "true" && os.Getenv("LIBVIRT_TEST") != "true" {
		t.Skip("skipping libvirt integration test in CI")
	}

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
	if os.Getenv("CI") == "true" && os.Getenv("LIBVIRT_TEST") != "true" {
		t.Skip("skipping libvirt integration test in CI")
	}

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
	if os.Getenv("CI") == "true" && os.Getenv("LIBVIRT_TEST") != "true" {
		t.Skip("skipping libvirt integration test in CI")
	}

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
	if os.Getenv("CI") == "true" && os.Getenv("LIBVIRT_TEST") != "true" {
		t.Skip("skipping libvirt integration test in CI")
	}

	conn, err := libvirt.NewConnect("qemu:///system")
	require.NoError(t, err)
	defer conn.Close()

	// Check for network that doesn't exist
	exists, err := NetworkExists(conn, "nonexistent-net-"+uuid.NewString())
	require.NoError(t, err)
	require.False(t, exists)
}
