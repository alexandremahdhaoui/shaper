//go:build integration

package network_test

import (
	"github.com/alexandremahdhaoui/shaper/pkg/network"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Integration tests

func TestStartDnsmasq_Integration(t *testing.T) {
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
	leaseFile := filepath.Join(tempDir, "dnsmasq.leases")
	tftpRoot := filepath.Join(tempDir, "tftp")

	// Create a test bridge first (dnsmasq needs an existing interface)
	bridgeName := "br" + strings.ReplaceAll(t.Name(), "/", "")[:8]
	if len(bridgeName) > 15 {
		bridgeName = bridgeName[:15]
	}

	bridgeConfig := network.BridgeConfig{
		Name: bridgeName,
		CIDR: "192.168.200.1/24",
	}
	err := network.CreateBridge(bridgeConfig)
	require.NoError(t, err)
	defer network.DeleteBridge(bridgeName)

	config := network.DnsmasqConfig{
		Interface:    bridgeName,
		DHCPRange:    "192.168.200.10,192.168.200.250",
		TFTPRoot:     tftpRoot,
		BootFilename: "test.boot",
		PIDFile:      pidFile,
		LeaseFile:    leaseFile,
		LogQueries:   true,
		LogDHCP:      true,
	}

	proc, err := network.StartDnsmasq(config, configPath)
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

	bridgeConfig := network.BridgeConfig{
		Name: bridgeName,
		CIDR: "192.168.201.1/24",
	}
	err := network.CreateBridge(bridgeConfig)
	require.NoError(t, err)
	defer network.DeleteBridge(bridgeName)

	config := network.DnsmasqConfig{
		Interface:    bridgeName,
		DHCPRange:    "192.168.201.10,192.168.201.250",
		TFTPRoot:     tftpRoot,
		BootFilename: "test.boot",
		PIDFile:      pidFile,
	}

	proc, err := network.StartDnsmasq(config, configPath)
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

	bridgeConfig := network.BridgeConfig{
		Name: bridgeName,
		CIDR: "192.168.202.1/24",
	}
	err := network.CreateBridge(bridgeConfig)
	require.NoError(t, err)
	defer network.DeleteBridge(bridgeName)

	config := network.DnsmasqConfig{
		Interface:    bridgeName,
		DHCPRange:    "192.168.202.10,192.168.202.250",
		TFTPRoot:     tftpRoot,
		BootFilename: "test.boot",
		PIDFile:      pidFile,
	}

	proc, err := network.StartDnsmasq(config, configPath)
	require.NoError(t, err)

	// Give dnsmasq a moment to start
	time.Sleep(100 * time.Millisecond)

	require.True(t, proc.IsRunning())

	// Stop using PID file
	err = network.StopByPIDFile(pidFile)
	require.NoError(t, err)

	// Give it a moment to stop
	time.Sleep(100 * time.Millisecond)

	// Verify process stopped
	require.False(t, proc.IsRunning())
}

