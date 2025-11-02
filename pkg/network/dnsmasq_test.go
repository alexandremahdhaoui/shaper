package network_test

import (
	"github.com/alexandremahdhaoui/shaper/pkg/network"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDnsmasqConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  network.DnsmasqConfig
		wantErr error
	}{
		{
			name: "valid config",
			config: network.DnsmasqConfig{
				Interface:    "br-test",
				DHCPRange:    "192.168.100.10,192.168.100.250",
				TFTPRoot:     "/tmp/tftp",
				BootFilename: "undionly.kpxe",
			},
			wantErr: nil,
		},
		{
			name: "missing interface",
			config: network.DnsmasqConfig{
				Interface:    "",
				DHCPRange:    "192.168.100.10,192.168.100.250",
				TFTPRoot:     "/tmp/tftp",
				BootFilename: "undionly.kpxe",
			},
			wantErr: network.ErrInterfaceRequired,
		},
		{
			name: "missing DHCP range",
			config: network.DnsmasqConfig{
				Interface:    "br-test",
				DHCPRange:    "",
				TFTPRoot:     "/tmp/tftp",
				BootFilename: "undionly.kpxe",
			},
			wantErr: network.ErrDHCPRangeRequired,
		},
		{
			name: "missing TFTP root",
			config: network.DnsmasqConfig{
				Interface:    "br-test",
				DHCPRange:    "192.168.100.10,192.168.100.250",
				TFTPRoot:     "",
				BootFilename: "undionly.kpxe",
			},
			wantErr: network.ErrTFTPRootRequired,
		},
		{
			name: "missing boot filename",
			config: network.DnsmasqConfig{
				Interface:    "br-test",
				DHCPRange:    "192.168.100.10,192.168.100.250",
				TFTPRoot:     "/tmp/tftp",
				BootFilename: "",
			},
			wantErr: network.ErrBootFilenameRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.config.GenerateConfig()
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
