package network_test

import (
	"github.com/alexandremahdhaoui/shaper/pkg/network"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLibvirtNetworkConfig_Validation(t *testing.T) {
	// These tests verify validation without requiring libvirt
	t.Run("nil connection", func(t *testing.T) {
		config := network.LibvirtNetworkConfig{
			Name: "test",
			Mode: "bridge",
		}
		err := network.CreateLibvirtNetwork(nil, config)
		require.ErrorIs(t, err, network.ErrConnNil)
	})

	t.Run("empty name", func(t *testing.T) {
		// We can't create a real connection for unit test
		// Just verify the error types are correct
		err := network.DeleteLibvirtNetwork(nil, "")
		require.ErrorIs(t, err, network.ErrNetworkNameRequired)

		exists, err := network.NetworkExists(nil, "")
		require.ErrorIs(t, err, network.ErrNetworkNameRequired)
		require.False(t, exists)
	})
}

func TestGenerateNetworkXML(t *testing.T) {
	tests := []struct {
		name       string
		config     network.LibvirtNetworkConfig
		shouldFail bool
		contains   []string
	}{
		{
			name: "bridge mode",
			config: network.LibvirtNetworkConfig{
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
			config: network.LibvirtNetworkConfig{
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
			config: network.LibvirtNetworkConfig{
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
			config: network.LibvirtNetworkConfig{
				Name: "test-fail",
				Mode: "bridge",
			},
			shouldFail: true,
		},
		{
			name: "unsupported mode",
			config: network.LibvirtNetworkConfig{
				Name: "test-fail",
				Mode: "invalid",
			},
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xml, err := network.GenerateNetworkXML(tt.config)
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
