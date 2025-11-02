//go:build integration

package network_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexandremahdhaoui/shaper/pkg/execcontext"
	"github.com/alexandremahdhaoui/shaper/pkg/network"
	"github.com/stretchr/testify/require"
)

// Integration tests for dnsmasq management

func TestDnsmasqManager_Create_Integration(t *testing.T) {
	// Check if dnsmasq is installed
	if _, err := exec.LookPath("dnsmasq"); err != nil {
		t.Skip("dnsmasq not installed")
	}

	execCtx := execcontext.New(nil, []string{"sudo"})
	bridgeMgr := network.NewBridgeManager(execCtx)
	dnsmasqMgr := network.NewDnsmasqManager(execCtx)
	ctx := context.Background()

	tempDir := t.TempDir()
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
	err := bridgeMgr.Create(ctx, bridgeConfig)
	require.NoError(t, err)
	defer bridgeMgr.Delete(ctx, bridgeName)

	// Create dnsmasq process
	dnsmasqID := "test-dnsmasq-1"
	config := network.DnsmasqConfig{
		Interface:    bridgeName,
		DHCPRange:    "192.168.200.10,192.168.200.250",
		TFTPRoot:     tftpRoot,
		BootFilename: "test.boot",
		LogQueries:   true,
		LogDHCP:      true,
	}

	err = dnsmasqMgr.Create(ctx, dnsmasqID, config)
	require.NoError(t, err)
	defer dnsmasqMgr.Delete(ctx, dnsmasqID)

	// Give dnsmasq a moment to start
	time.Sleep(100 * time.Millisecond)

	// Verify process is running using Get
	info, err := dnsmasqMgr.Get(ctx, dnsmasqID)
	require.NoError(t, err)
	require.NotNil(t, info)
	require.True(t, info.IsRunning)

	// Verify TFTP root was created
	require.DirExists(t, tftpRoot)
}

func TestDnsmasqManager_Delete_Integration(t *testing.T) {
	// Check if dnsmasq is installed
	if _, err := exec.LookPath("dnsmasq"); err != nil {
		t.Skip("dnsmasq not installed")
	}

	execCtx := execcontext.New(nil, []string{"sudo"})
	bridgeMgr := network.NewBridgeManager(execCtx)
	dnsmasqMgr := network.NewDnsmasqManager(execCtx)
	ctx := context.Background()

	tempDir := t.TempDir()
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
	err := bridgeMgr.Create(ctx, bridgeConfig)
	require.NoError(t, err)
	defer bridgeMgr.Delete(ctx, bridgeName)

	// Create dnsmasq process
	dnsmasqID := "test-dnsmasq-2"
	config := network.DnsmasqConfig{
		Interface:    bridgeName,
		DHCPRange:    "192.168.201.10,192.168.201.250",
		TFTPRoot:     tftpRoot,
		BootFilename: "test.boot",
	}

	err = dnsmasqMgr.Create(ctx, dnsmasqID, config)
	require.NoError(t, err)

	// Give dnsmasq a moment to start
	time.Sleep(100 * time.Millisecond)

	// Verify it's running
	info, err := dnsmasqMgr.Get(ctx, dnsmasqID)
	require.NoError(t, err)
	require.True(t, info.IsRunning)

	// Delete dnsmasq
	err = dnsmasqMgr.Delete(ctx, dnsmasqID)
	require.NoError(t, err)

	// Give it a moment to stop
	time.Sleep(100 * time.Millisecond)

	// Verify it's gone (Get should return ErrDnsmasqNotFound)
	_, err = dnsmasqMgr.Get(ctx, dnsmasqID)
	require.Error(t, err)
}

func TestDnsmasqManager_Delete_Idempotent_Integration(t *testing.T) {
	// Check if dnsmasq is installed
	if _, err := exec.LookPath("dnsmasq"); err != nil {
		t.Skip("dnsmasq not installed")
	}

	execCtx := execcontext.New(nil, []string{"sudo"})
	dnsmasqMgr := network.NewDnsmasqManager(execCtx)
	ctx := context.Background()

	// Delete non-existent process should not error
	err := dnsmasqMgr.Delete(ctx, "non-existent-dnsmasq")
	require.NoError(t, err)
}
