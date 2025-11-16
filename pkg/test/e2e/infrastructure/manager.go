//go:build e2e

package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alexandremahdhaoui/shaper/pkg/execcontext"
	"github.com/alexandremahdhaoui/shaper/pkg/network"
	"github.com/alexandremahdhaoui/shaper/pkg/test/kind"
	"github.com/alexandremahdhaoui/shaper/pkg/vmm"
	"github.com/google/uuid"
)

// Sentinel errors for infrastructure operations
var (
	ErrBridgeCreationFailed = errors.New("bridge creation failed")
	ErrDnsmasqStartFailed   = errors.New("dnsmasq start failed")
	ErrLibvirtNetworkFailed = errors.New("libvirt network creation failed")
	ErrKindClusterFailed    = errors.New("KIND cluster creation failed")
	ErrDeploymentFailed     = errors.New("shaper deployment failed")
	ErrEnvironmentNotFound  = errors.New("environment not found")
	ErrInvalidSpec          = errors.New("invalid infrastructure spec")
	ErrCleanupFailed        = errors.New("cleanup failed")
)

// InfrastructureSpec defines infrastructure requirements
// This matches the schema from architecture.md
type InfrastructureSpec struct {
	Network NetworkSpec
	Kind    KindSpec
	Shaper  ShaperSpec
}

// NetworkSpec defines network configuration
type NetworkSpec struct {
	CIDR      string
	Bridge    string
	DHCPRange string
}

// KindSpec defines KIND cluster configuration
type KindSpec struct {
	ClusterName string
	Version     string
}

// ShaperSpec defines shaper deployment configuration
type ShaperSpec struct {
	Namespace   string
	APIReplicas int
}

// InfrastructureState represents the complete state of provisioned infrastructure
// This type MUST match architecture.md specification
type InfrastructureState struct {
	ID             string
	BridgeName     string
	LibvirtNetwork string
	DnsmasqID      string
	KindCluster    string
	Kubeconfig     string
	TFTPRoot       string
	ArtifactDir    string

	// Timestamps
	CreatedAt time.Time
}

// InfrastructureManager manages test infrastructure lifecycle
type InfrastructureManager struct {
	config      InfrastructureSpec
	artifactDir string
}

// NewInfrastructureManager creates a new infrastructure manager
func NewInfrastructureManager(spec InfrastructureSpec, artifactDir string) *InfrastructureManager {
	return &InfrastructureManager{
		config:      spec,
		artifactDir: artifactDir,
	}
}

// Setup provisions all infrastructure based on scenario spec
// Returns InfrastructureState containing IDs and paths for cleanup
func (m *InfrastructureManager) Setup(ctx context.Context) (*InfrastructureState, error) {
	// Validate spec
	if err := m.validateSpec(); err != nil {
		return nil, errors.Join(err, ErrInvalidSpec)
	}

	// Generate unique test ID with timestamp
	testID := generateTestID()

	state := &InfrastructureState{
		ID:          testID,
		BridgeName:  m.config.Network.Bridge,
		KindCluster: m.config.Kind.ClusterName,
		CreatedAt:   time.Now(),
	}

	// Create artifact directory for this test
	state.ArtifactDir = filepath.Join(m.artifactDir, testID)
	if err := os.MkdirAll(state.ArtifactDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create artifact directory: %w", err)
	}

	// Create temp directory for TFTP root
	tempDirRoot := filepath.Join(os.TempDir(), testID)
	if err := os.MkdirAll(tempDirRoot, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	state.TFTPRoot = filepath.Join(tempDirRoot, "tftp")
	if err := os.MkdirAll(state.TFTPRoot, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create TFTP root: %w", err)
	}

	// Setup infrastructure in order
	// Use sudo context for network operations
	execCtx := execcontext.New(nil, []string{"sudo"})

	// Step 1: Create network bridge
	if err := m.createBridge(ctx, execCtx, state); err != nil {
		// Cleanup on failure
		_ = m.Teardown(ctx, state)
		return nil, errors.Join(err, ErrBridgeCreationFailed)
	}

	// Step 2: Create libvirt network
	if err := m.createLibvirtNetwork(ctx, state); err != nil {
		_ = m.Teardown(ctx, state)
		return nil, errors.Join(err, ErrLibvirtNetworkFailed)
	}

	// Step 3: Start dnsmasq
	if err := m.startDnsmasq(ctx, execCtx, state); err != nil {
		_ = m.Teardown(ctx, state)
		return nil, errors.Join(err, ErrDnsmasqStartFailed)
	}

	// Step 4: Create KIND cluster
	if err := m.createKindCluster(ctx, state); err != nil {
		_ = m.Teardown(ctx, state)
		return nil, errors.Join(err, ErrKindClusterFailed)
	}

	// Step 5: Deploy shaper components
	if err := m.deployShaper(ctx, state); err != nil {
		_ = m.Teardown(ctx, state)
		return nil, errors.Join(err, ErrDeploymentFailed)
	}

	return state, nil
}

// Teardown cleans up all infrastructure resources
// Continues cleanup even if individual steps fail, collecting all errors
func (m *InfrastructureManager) Teardown(ctx context.Context, state *InfrastructureState) error {
	if state == nil {
		return nil
	}

	var errs []error
	execCtx := execcontext.New(nil, []string{"sudo"})

	// Stop dnsmasq
	if state.DnsmasqID != "" {
		dnsmasqMgr := network.NewDnsmasqManager(execCtx)
		if err := dnsmasqMgr.Delete(ctx, state.DnsmasqID); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop dnsmasq %s: %w", state.DnsmasqID, err))
		}
	}

	// Delete libvirt network
	if state.LibvirtNetwork != "" {
		conn, err := vmm.NewVMM()
		if err == nil {
			defer func() { _ = conn.Close() }()
			libvirtMgr := network.NewLibvirtNetworkManager(conn.GetConnection())
			if err := libvirtMgr.Delete(ctx, state.LibvirtNetwork); err != nil {
				errs = append(errs, fmt.Errorf("failed to delete libvirt network %s: %w", state.LibvirtNetwork, err))
			}
		} else {
			errs = append(errs, fmt.Errorf("failed to connect to libvirt for network cleanup: %w", err))
		}
	}

	// Delete KIND cluster
	if state.KindCluster != "" {
		if err := kind.DeleteCluster(state.KindCluster); err != nil {
			errs = append(errs, fmt.Errorf("failed to delete KIND cluster %s: %w", state.KindCluster, err))
		}
	}

	// Delete network bridge
	if state.BridgeName != "" {
		bridgeMgr := network.NewBridgeManager(execCtx)
		if err := bridgeMgr.Delete(ctx, state.BridgeName); err != nil {
			errs = append(errs, fmt.Errorf("failed to delete bridge %s: %w", state.BridgeName, err))
		}
	}

	// Remove temp directories (TFTP root)
	if state.TFTPRoot != "" {
		tempDirRoot := filepath.Dir(state.TFTPRoot)
		if err := os.RemoveAll(tempDirRoot); err != nil {
			errs = append(errs, fmt.Errorf("failed to remove temp directory %s: %w", tempDirRoot, err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(append([]error{ErrCleanupFailed}, errs...)...)
	}

	return nil
}

// GetState retrieves infrastructure state by ID
// Currently not implemented as state persistence is not in this task's scope
func (m *InfrastructureManager) GetState(id string) (*InfrastructureState, error) {
	// TODO: Implement state persistence in future task
	return nil, ErrEnvironmentNotFound
}

// validateSpec validates the infrastructure spec
func (m *InfrastructureManager) validateSpec() error {
	if m.config.Network.CIDR == "" {
		return fmt.Errorf("network CIDR is required")
	}
	if m.config.Network.Bridge == "" {
		return fmt.Errorf("network bridge name is required")
	}
	if m.config.Network.DHCPRange == "" {
		return fmt.Errorf("network DHCP range is required")
	}
	if m.config.Kind.ClusterName == "" {
		return fmt.Errorf("KIND cluster name is required")
	}
	if m.config.Shaper.Namespace == "" {
		return fmt.Errorf("shaper namespace is required")
	}
	return nil
}

// createBridge creates the network bridge
func (m *InfrastructureManager) createBridge(ctx context.Context, execCtx execcontext.Context, state *InfrastructureState) error {
	bridgeMgr := network.NewBridgeManager(execCtx)

	bridgeConfig := network.BridgeConfig{
		Name: m.config.Network.Bridge,
		CIDR: m.config.Network.CIDR,
	}

	if err := bridgeMgr.Create(ctx, bridgeConfig); err != nil {
		return fmt.Errorf("failed to create bridge %s: %w", m.config.Network.Bridge, err)
	}

	return nil
}

// createLibvirtNetwork creates the libvirt network
func (m *InfrastructureManager) createLibvirtNetwork(ctx context.Context, state *InfrastructureState) error {
	conn, err := vmm.NewVMM()
	if err != nil {
		return fmt.Errorf("failed to connect to libvirt: %w", err)
	}
	defer func() { _ = conn.Close() }()

	libvirtNetworkName := "net-" + state.ID
	state.LibvirtNetwork = libvirtNetworkName

	libvirtMgr := network.NewLibvirtNetworkManager(conn.GetConnection())
	libvirtNetConfig := network.LibvirtNetworkConfig{
		Name:       libvirtNetworkName,
		BridgeName: m.config.Network.Bridge,
		Mode:       "bridge",
	}

	if err := libvirtMgr.Create(ctx, libvirtNetConfig); err != nil {
		return fmt.Errorf("failed to create libvirt network %s: %w", libvirtNetworkName, err)
	}

	return nil
}

// startDnsmasq starts the dnsmasq service
func (m *InfrastructureManager) startDnsmasq(ctx context.Context, execCtx execcontext.Context, state *InfrastructureState) error {
	dnsmasqID := "dnsmasq-" + state.ID
	state.DnsmasqID = dnsmasqID

	dnsmasqMgr := network.NewDnsmasqManager(execCtx)
	dnsmasqConfig := network.DnsmasqConfig{
		Interface:    m.config.Network.Bridge,
		DHCPRange:    m.config.Network.DHCPRange,
		TFTPRoot:     state.TFTPRoot,
		BootFilename: "undionly.kpxe", // Default iPXE boot file
		LogQueries:   true,
		LogDHCP:      true,
	}

	if err := dnsmasqMgr.Create(ctx, dnsmasqID, dnsmasqConfig); err != nil {
		return fmt.Errorf("failed to start dnsmasq %s: %w", dnsmasqID, err)
	}

	return nil
}

// createKindCluster creates the KIND cluster
func (m *InfrastructureManager) createKindCluster(ctx context.Context, state *InfrastructureState) error {
	kubeconfigPath := filepath.Join(state.ArtifactDir, "kubeconfig")
	state.Kubeconfig = kubeconfigPath

	kindConfig := kind.ClusterConfig{
		Name:       m.config.Kind.ClusterName,
		Kubeconfig: kubeconfigPath,
	}

	if err := kind.CreateCluster(kindConfig); err != nil {
		return fmt.Errorf("failed to create KIND cluster %s: %w", m.config.Kind.ClusterName, err)
	}

	return nil
}

// deployShaper deploys shaper components to KIND cluster
func (m *InfrastructureManager) deployShaper(ctx context.Context, state *InfrastructureState) error {
	// Deploy shaper CRDs and components
	// Find CRD paths (assuming standard project layout)
	crdPaths := []string{
		"charts/shaper-crds/templates/crds/",
	}

	deployConfig := kind.DeployConfig{
		Kubeconfig:  state.Kubeconfig,
		Namespace:   m.config.Shaper.Namespace,
		CRDPaths:    crdPaths,
		WaitTimeout: 2 * time.Minute,
	}

	if err := kind.DeployShaperToKIND(deployConfig); err != nil {
		// Don't fail if deployment fails - CRDs might already be deployed
		// Just log the warning (caller should check if this is acceptable)
		return fmt.Errorf("warning: shaper deployment had issues: %w", err)
	}

	return nil
}

// generateTestID generates a unique test ID with format: e2e-<timestamp>-<random>
func generateTestID() string {
	timestamp := time.Now().Format("20060102-150405")
	random := uuid.NewString()[:8]
	return fmt.Sprintf("e2e-%s-%s", timestamp, random)
}
