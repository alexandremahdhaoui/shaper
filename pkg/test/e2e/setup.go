//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/alexandremahdhaoui/shaper/pkg/execcontext"
	"github.com/alexandremahdhaoui/shaper/pkg/network"
	"github.com/alexandremahdhaoui/shaper/pkg/test/kind"
	"github.com/alexandremahdhaoui/shaper/pkg/vmm"
	"github.com/alexandremahdhaoui/tooling/pkg/flaterrors"
	"github.com/google/uuid"
)

// ShaperSetupConfig contains configuration for shaper test environment setup
type ShaperSetupConfig struct {
	// Base config
	ArtifactDir   string
	ImageCacheDir string

	// Network config
	BridgeName  string // e.g., "br-shaper"
	NetworkCIDR string // e.g., "192.168.100.1/24"
	DHCPRange   string // e.g., "192.168.100.10,192.168.100.250"

	// KIND config
	KindClusterName string

	// Shaper deployment (optional - can deploy manually)
	CRDPaths       []string
	DeploymentPath string

	// PXE boot files
	TFTPRoot     string
	IPXEBootFile string // Path to undionly.kpxe or ipxe.efi

	// Client VMs
	NumClients      int
	ClientMemoryMB  uint
	ClientVCPUs     uint
	ClientImagePath string // Path to client VM image

	// Download images if missing
	DownloadImages bool
}

// ShaperTestEnvironment extends TestEnvironment with shaper-specific fields
type ShaperTestEnvironment struct {
	// Unique test ID
	ID string

	// Network infrastructure
	BridgeName     string
	LibvirtNetwork string
	DnsmasqID      string // ID for dnsmasq manager

	// KIND cluster
	KindCluster     string
	Kubeconfig      string
	ShaperNamespace string

	// Client VMs (not started yet)
	ClientVMs []*vmm.VMMetadata

	// Paths
	ArtifactPath string
	TempDirRoot  string
	TFTPRoot     string

	// Cleanup tracking
	ManagedResources []string
	TempDirs         []string
}

// SetupShaperTestEnvironment creates complete shaper test environment
func SetupShaperTestEnvironment(config ShaperSetupConfig) (*ShaperTestEnvironment, error) {
	// Generate unique test ID
	testID := "e2e-shaper-" + uuid.NewString()[:8]

	env := &ShaperTestEnvironment{
		ID:              testID,
		BridgeName:      config.BridgeName,
		KindCluster:     config.KindClusterName,
		ShaperNamespace: "default",
	}

	// Create artifact directory
	env.ArtifactPath = filepath.Join(config.ArtifactDir, testID)
	if err := os.MkdirAll(env.ArtifactPath, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create artifact directory: %v", err)
	}

	// Create temp directory root
	env.TempDirRoot = filepath.Join(os.TempDir(), testID)
	if err := os.MkdirAll(env.TempDirRoot, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %v", err)
	}
	env.TempDirs = append(env.TempDirs, env.TempDirRoot)

	// Set TFTP root
	if config.TFTPRoot != "" {
		env.TFTPRoot = config.TFTPRoot
	} else {
		env.TFTPRoot = filepath.Join(env.TempDirRoot, "tftp")
	}

	ctx := context.Background()

	// Step 1: Create network bridge
	// Use sudo context for network operations
	execCtx := execcontext.New(nil, []string{"sudo"})
	bridgeMgr := network.NewBridgeManager(execCtx)

	bridgeConfig := network.BridgeConfig{
		Name: config.BridgeName,
		CIDR: config.NetworkCIDR,
	}
	if err := bridgeMgr.Create(ctx, bridgeConfig); err != nil {
		return nil, flaterrors.Join(err, fmt.Errorf("failed to create bridge"))
	}

	// Step 2: Create libvirt network (using the bridge)
	libvirtNetworkName := "net-" + testID
	env.LibvirtNetwork = libvirtNetworkName

	conn, err := vmm.NewVMM()
	if err != nil {
		return nil, flaterrors.Join(err, fmt.Errorf("failed to create VMM"))
	}
	defer conn.Close()

	libvirtMgr := network.NewLibvirtNetworkManager(conn.GetConnection())
	libvirtNetConfig := network.LibvirtNetworkConfig{
		Name:       libvirtNetworkName,
		BridgeName: config.BridgeName,
		Mode:       "bridge",
	}
	if err := libvirtMgr.Create(ctx, libvirtNetConfig); err != nil {
		return nil, flaterrors.Join(err, fmt.Errorf("failed to create libvirt network"))
	}

	// Step 3: Start dnsmasq
	dnsmasqID := "dnsmasq-" + testID
	env.DnsmasqID = dnsmasqID

	dnsmasqMgr := network.NewDnsmasqManager(execCtx)
	dnsmasqConfig := network.DnsmasqConfig{
		Interface:    config.BridgeName,
		DHCPRange:    config.DHCPRange,
		TFTPRoot:     env.TFTPRoot,
		BootFilename: filepath.Base(config.IPXEBootFile),
		LogQueries:   true,
		LogDHCP:      true,
	}

	if err := dnsmasqMgr.Create(ctx, dnsmasqID, dnsmasqConfig); err != nil {
		return nil, flaterrors.Join(err, fmt.Errorf("failed to start dnsmasq"))
	}

	// Copy iPXE boot file to TFTP root if provided
	if config.IPXEBootFile != "" && fileExists(config.IPXEBootFile) {
		destPath := filepath.Join(env.TFTPRoot, filepath.Base(config.IPXEBootFile))
		if err := copyFile(config.IPXEBootFile, destPath); err != nil {
			return nil, flaterrors.Join(err, fmt.Errorf("failed to copy iPXE boot file"))
		}
	}

	// Step 4: Create KIND cluster
	kubeconfigPath := filepath.Join(env.ArtifactPath, "kubeconfig")
	env.Kubeconfig = kubeconfigPath

	kindConfig := kind.ClusterConfig{
		Name:       config.KindClusterName,
		Kubeconfig: kubeconfigPath,
	}
	if err := kind.CreateCluster(kindConfig); err != nil {
		return nil, flaterrors.Join(err, fmt.Errorf("failed to create KIND cluster"))
	}

	// Step 5: Deploy shaper to KIND (if CRDs/deployment provided)
	if len(config.CRDPaths) > 0 || config.DeploymentPath != "" {
		deployConfig := kind.DeployConfig{
			Kubeconfig:     kubeconfigPath,
			Namespace:      env.ShaperNamespace,
			CRDPaths:       config.CRDPaths,
			DeploymentPath: config.DeploymentPath,
			WaitTimeout:    2 * time.Minute,
		}
		if err := kind.DeployShaperToKIND(deployConfig); err != nil {
			// Don't fail if deployment fails - might be deployed manually
			// Just log the error
			fmt.Printf("Warning: failed to deploy shaper: %v\n", err)
		}
	}

	// Step 6: Create client VMs (not started)
	if config.NumClients > 0 {
		// Download client image if needed
		clientImagePath := config.ClientImagePath
		if clientImagePath == "" && config.DownloadImages {
			// Use default Ubuntu image
			clientImagePath = filepath.Join(config.ImageCacheDir, "ubuntu-24.04-server-cloudimg-amd64.img")
			if !fileExists(clientImagePath) {
				if err := downloadVMImage(
					"https://cloud-images.ubuntu.com/releases/noble/release/ubuntu-24.04-server-cloudimg-amd64.img",
					clientImagePath,
				); err != nil {
					return nil, flaterrors.Join(err, fmt.Errorf("failed to download client image"))
				}
			}
		}

		// Create client VMs
		for i := 0; i < config.NumClients; i++ {
			vmName := fmt.Sprintf("client-%s-%d", testID, i)

			// For now, just track metadata - don't actually create VMs yet
			// They will be created when tests run
			metadata := &vmm.VMMetadata{
				Name:     vmName,
				MemoryMB: config.ClientMemoryMB,
				VCPUs:    config.ClientVCPUs,
			}
			env.ClientVMs = append(env.ClientVMs, metadata)
		}
	}

	return env, nil
}

// TeardownShaperTestEnvironment cleans up test environment
func TeardownShaperTestEnvironment(env *ShaperTestEnvironment) error {
	if env == nil {
		return nil
	}

	var errors []error
	ctx := context.Background()
	execCtx := execcontext.New(nil, []string{"sudo"})

	// Stop dnsmasq using manager
	if env.DnsmasqID != "" {
		dnsmasqMgr := network.NewDnsmasqManager(execCtx)
		if err := dnsmasqMgr.Delete(ctx, env.DnsmasqID); err != nil {
			errors = append(errors, fmt.Errorf("failed to stop dnsmasq: %v", err))
		}
	}

	// Delete client VMs if they exist
	if len(env.ClientVMs) > 0 {
		vmmConn, err := vmm.NewVMM()
		if err == nil {
			defer vmmConn.Close()
			for _, vm := range env.ClientVMs {
				if err := vmmConn.DestroyVM(execCtx, vm.Name); err != nil {
					errors = append(errors, fmt.Errorf("failed to destroy VM %s: %v", vm.Name, err))
				}
			}
		}
	}

	// Delete libvirt network using manager
	if env.LibvirtNetwork != "" {
		vmmConn, err := vmm.NewVMM()
		if err == nil {
			defer vmmConn.Close()
			libvirtMgr := network.NewLibvirtNetworkManager(vmmConn.GetConnection())
			if err := libvirtMgr.Delete(ctx, env.LibvirtNetwork); err != nil {
				errors = append(errors, fmt.Errorf("failed to delete libvirt network: %v", err))
			}
		}
	}

	// Delete KIND cluster
	if env.KindCluster != "" {
		if err := kind.DeleteCluster(env.KindCluster); err != nil {
			errors = append(errors, fmt.Errorf("failed to delete KIND cluster: %v", err))
		}
	}

	// Delete network bridge using manager
	if env.BridgeName != "" {
		bridgeMgr := network.NewBridgeManager(execCtx)
		if err := bridgeMgr.Delete(ctx, env.BridgeName); err != nil {
			errors = append(errors, fmt.Errorf("failed to delete bridge: %v", err))
		}
	}

	// Remove temp directories
	for _, dir := range env.TempDirs {
		if err := os.RemoveAll(dir); err != nil {
			errors = append(errors, fmt.Errorf("failed to remove temp dir %s: %v", dir, err))
		}
	}

	if len(errors) > 0 {
		return flaterrors.Join(errors...)
	}

	return nil
}

// Helper functions

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func downloadVMImage(url, destPath string) error {
	// Simple download using wget
	// Ensure directory exists
	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	cmd := exec.Command("wget", "-O", destPath, url)
	if err := cmd.Run(); err != nil {
		// Clean up partial file
		os.Remove(destPath)
		return fmt.Errorf("failed to download image: %v", err)
	}

	return nil
}
