package network

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDnsmasqConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  DnsmasqConfig
		wantErr error
	}{
		{
			name: "valid config",
			config: DnsmasqConfig{
				Interface:    "br-test",
				DHCPRange:    "192.168.100.10,192.168.100.250",
				TFTPRoot:     "/tmp/tftp",
				BootFilename: "undionly.kpxe",
			},
			wantErr: nil,
		},
		{
			name: "missing interface",
			config: DnsmasqConfig{
				Interface:    "",
				DHCPRange:    "192.168.100.10,192.168.100.250",
				TFTPRoot:     "/tmp/tftp",
				BootFilename: "undionly.kpxe",
			},
			wantErr: errInterfaceRequired,
		},
		{
			name: "missing DHCP range",
			config: DnsmasqConfig{
				Interface:    "br-test",
				DHCPRange:    "",
				TFTPRoot:     "/tmp/tftp",
				BootFilename: "undionly.kpxe",
			},
			wantErr: errDHCPRangeRequired,
		},
		{
			name: "missing TFTP root",
			config: DnsmasqConfig{
				Interface:    "br-test",
				DHCPRange:    "192.168.100.10,192.168.100.250",
				TFTPRoot:     "",
				BootFilename: "undionly.kpxe",
			},
			wantErr: errTFTPRootRequired,
		},
		{
			name: "missing boot filename",
			config: DnsmasqConfig{
				Interface:    "br-test",
				DHCPRange:    "192.168.100.10,192.168.100.250",
				TFTPRoot:     "/tmp/tftp",
				BootFilename: "",
			},
			wantErr: errBootFilenameRequired,
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

func TestDnsmasqConfig_GenerateConfig(t *testing.T) {
	config := DnsmasqConfig{
		Interface:    "br-test",
		DHCPRange:    "192.168.100.10,192.168.100.250",
		TFTPRoot:     "/tmp/tftp",
		BootFilename: "undionly.kpxe",
		PIDFile:      "/tmp/dnsmasq.pid",
		LeaseFile:    "/tmp/dnsmasq.leases",
		LogQueries:   true,
		LogDHCP:      true,
		DNSServers:   []string{"8.8.8.8", "8.8.4.4"},
	}

	content, err := config.GenerateConfig()
	require.NoError(t, err)

	configStr := string(content)
	// Verify key configuration items are present
	require.Contains(t, configStr, "interface=br-test")
	require.Contains(t, configStr, "dhcp-range=192.168.100.10,192.168.100.250")
	require.Contains(t, configStr, "tftp-root=/tmp/tftp")
	require.Contains(t, configStr, "dhcp-boot=undionly.kpxe")
	require.Contains(t, configStr, "pid-file=/tmp/dnsmasq.pid")
	require.Contains(t, configStr, "dhcp-leasefile=/tmp/dnsmasq.leases")
	require.Contains(t, configStr, "log-queries")
	require.Contains(t, configStr, "log-dhcp")
	require.Contains(t, configStr, "server=8.8.8.8")
	require.Contains(t, configStr, "server=8.8.4.4")
}

func TestDnsmasqConfig_WriteConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "dnsmasq.conf")

	config := DnsmasqConfig{
		Interface:    "br-test",
		DHCPRange:    "192.168.100.10,192.168.100.250",
		TFTPRoot:     "/tmp/tftp",
		BootFilename: "undionly.kpxe",
	}

	err := config.WriteConfig(configPath)
	require.NoError(t, err)

	// Verify file was created
	require.FileExists(t, configPath)

	// Verify content
	content, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Contains(t, string(content), "interface=br-test")
}



	if os.Geteuid() != 0 {
		t.Skip("requires root privileges")
	}

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "dnsmasq.conf")
	pidFile := filepath.Join(tempDir, "dnsmasq.pid")
	leaseFile := filepath.Join(tempDir, "dnsmasq.leases")
	tftpRoot := filepath.Join(tempDir, "tftp")

	// Create a test bridge first (dnsmasq needs an existing interface)
	bridgeName := "br" + strings.ReplaceAll(t.Name(), "/", "")[:8]
	if len(bridgeName) > 15 {
		bridgeName = bridgeName[:15]
	}

	bridgeConfig := BridgeConfig{
		Name: bridgeName,
		CIDR: "192.168.200.1/24",
	}
	err := CreateBridge(bridgeConfig)
	require.NoError(t, err)
	defer DeleteBridge(bridgeName)

	config := DnsmasqConfig{
		Interface:    bridgeName,
		DHCPRange:    "192.168.200.10,192.168.200.250",
		TFTPRoot:     tftpRoot,
		BootFilename: "test.boot",
		PIDFile:      pidFile,
		LeaseFile:    leaseFile,
		LogQueries:   true,
		LogDHCP:      true,
	}

	proc, err := StartDnsmasq(config, configPath)
	require.NoError(t, err)
	require.NotNil(t, proc)
	defer proc.Stop()

	// Give dnsmasq a moment to start
	time.Sleep(100 * time.Millisecond)

	// Verify process is running
	require.True(t, proc.IsRunning())

	// Verify PID file was created
	require.FileExists(t, pidFile)

	// Verify TFTP root was created
	require.DirExists(t, tftpRoot)
}

func TestStopDnsmasq_Integration(t *testing.T) {
	// Check if dnsmasq is installed
	if _, err := exec.LookPath("dnsmasq"); err != nil {
		t.Skip("dnsmasq not installed")
	}

	if os.Geteuid() != 0 {
		t.Skip("requires root privileges")
	}

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "dnsmasq.conf")
	pidFile := filepath.Join(tempDir, "dnsmasq.pid")
	tftpRoot := filepath.Join(tempDir, "tftp")

	// Create a test bridge
	bridgeName := "br" + strings.ReplaceAll(t.Name(), "/", "")[:8]
	if len(bridgeName) > 15 {
		bridgeName = bridgeName[:15]
	}

	bridgeConfig := BridgeConfig{
		Name: bridgeName,
		CIDR: "192.168.201.1/24",
	}
	err := CreateBridge(bridgeConfig)
	require.NoError(t, err)
	defer DeleteBridge(bridgeName)

	config := DnsmasqConfig{
		Interface:    bridgeName,
		DHCPRange:    "192.168.201.10,192.168.201.250",
		TFTPRoot:     tftpRoot,
		BootFilename: "test.boot",
		PIDFile:      pidFile,
	}

	proc, err := StartDnsmasq(config, configPath)
	require.NoError(t, err)

	// Give dnsmasq a moment to start
	time.Sleep(100 * time.Millisecond)

	require.True(t, proc.IsRunning())

	// Stop dnsmasq
	err = proc.Stop()
	require.NoError(t, err)

	// Give it a moment to stop
	time.Sleep(100 * time.Millisecond)

	// Verify process stopped
	require.False(t, proc.IsRunning())
}

func TestStopByPIDFile_Integration(t *testing.T) {
	// Check if dnsmasq is installed
	if _, err := exec.LookPath("dnsmasq"); err != nil {
		t.Skip("dnsmasq not installed")
	}

	if os.Geteuid() != 0 {
		t.Skip("requires root privileges")
	}

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "dnsmasq.conf")
	pidFile := filepath.Join(tempDir, "dnsmasq.pid")
	tftpRoot := filepath.Join(tempDir, "tftp")

	// Create a test bridge
	bridgeName := "br" + strings.ReplaceAll(t.Name(), "/", "")[:8]
	if len(bridgeName) > 15 {
		bridgeName = bridgeName[:15]
	}

	bridgeConfig := BridgeConfig{
		Name: bridgeName,
		CIDR: "192.168.202.1/24",
	}
	err := CreateBridge(bridgeConfig)
	require.NoError(t, err)
	defer DeleteBridge(bridgeName)

	config := DnsmasqConfig{
		Interface:    bridgeName,
		DHCPRange:    "192.168.202.10,192.168.202.250",
		TFTPRoot:     tftpRoot,
		BootFilename: "test.boot",
		PIDFile:      pidFile,
	}

	proc, err := StartDnsmasq(config, configPath)
	require.NoError(t, err)

	// Give dnsmasq a moment to start
	time.Sleep(100 * time.Millisecond)

	require.True(t, proc.IsRunning())

	// Stop using PID file
	err = StopByPIDFile(pidFile)
	require.NoError(t, err)

	// Give it a moment to stop
	time.Sleep(100 * time.Millisecond)

	// Verify process stopped
	require.False(t, proc.IsRunning())
}

func TestStopByPIDFile_NonExistent(t *testing.T) {
	// Stopping non-existent PID file should not error
	err := StopByPIDFile("/tmp/nonexistent-pid-file-12345.pid")
	require.NoError(t, err)
}
