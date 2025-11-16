//go:build e2e

package forge

import (
	"context"
	"errors"
	"fmt"

	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e/infrastructure"
	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e/scenario"
)

var (
	// ErrInvalidConfig indicates the provided configuration is invalid
	ErrInvalidConfig = errors.New("invalid config")
	// ErrSetupFailed indicates infrastructure setup failed
	ErrSetupFailed = errors.New("setup failed")
	// ErrTeardownFailed indicates infrastructure teardown failed
	ErrTeardownFailed = errors.New("teardown failed")
)

// Testenv manages E2E test environment lifecycle for forge
type Testenv struct {
	storeDir    string
	artifactDir string
	store       EnvironmentStore
}

// NewTestenv creates a new Testenv instance
func NewTestenv(storeDir, artifactDir string) (*Testenv, error) {
	store, err := NewJSONEnvironmentStore(storeDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create store: %w", err)
	}

	return &Testenv{
		storeDir:    storeDir,
		artifactDir: artifactDir,
		store:       store,
	}, nil
}

// Create provisions a new test environment
// Config should contain:
//   - scenarioPath (optional): path to scenario YAML file
//   - network.cidr: network CIDR (e.g., "192.168.100.1/24")
//   - network.bridge: bridge name (e.g., "br-shaper-e2e")
//   - network.dhcpRange: DHCP range (e.g., "192.168.100.10,192.168.100.100")
//   - kind.clusterName: KIND cluster name (e.g., "shaper-e2e")
//   - kind.version: Kubernetes version (optional)
//   - shaper.namespace: namespace for shaper (e.g., "default")
//   - shaper.apiReplicas: number of API replicas (default: 1)
func (t *Testenv) Create(ctx context.Context, config map[string]interface{}) (string, error) {
	// Parse config into InfrastructureSpec
	spec, err := t.parseConfig(config)
	if err != nil {
		return "", errors.Join(err, ErrInvalidConfig)
	}

	// Create infrastructure manager
	mgr := infrastructure.NewInfrastructureManager(spec, t.artifactDir)

	// Setup infrastructure
	state, err := mgr.Setup(ctx)
	if err != nil {
		return "", errors.Join(err, ErrSetupFailed)
	}

	// Save state to store
	if err := t.store.Save(state); err != nil {
		// Try to cleanup on save failure
		_ = mgr.Teardown(ctx, state)
		return "", fmt.Errorf("failed to save environment state: %w", err)
	}

	return state.ID, nil
}

// Get retrieves environment details by ID
// Returns a map with infrastructure details
func (t *Testenv) Get(ctx context.Context, id string) (map[string]interface{}, error) {
	// Load state from store
	state, err := t.store.Load(id)
	if err != nil {
		return nil, err
	}

	// Convert state to map
	result := map[string]interface{}{
		"id":             state.ID,
		"bridgeName":     state.BridgeName,
		"libvirtNetwork": state.LibvirtNetwork,
		"dnsmasqID":      state.DnsmasqID,
		"kindCluster":    state.KindCluster,
		"kubeconfig":     state.Kubeconfig,
		"tftpRoot":       state.TFTPRoot,
		"artifactDir":    state.ArtifactDir,
		"createdAt":      state.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	return result, nil
}

// List returns all test environments
// Returns a slice of maps with summary information
func (t *Testenv) List(ctx context.Context) ([]map[string]interface{}, error) {
	// Load all environments from store
	environments, err := t.store.List()
	if err != nil {
		return nil, err
	}

	// Convert to maps
	result := make([]map[string]interface{}, len(environments))
	for i, env := range environments {
		result[i] = map[string]interface{}{
			"id":          env.ID,
			"kindCluster": env.KindCluster,
			"createdAt":   env.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	return result, nil
}

// Delete cleans up a test environment
func (t *Testenv) Delete(ctx context.Context, id string) error {
	// Load state from store
	state, err := t.store.Load(id)
	if err != nil {
		return err
	}

	// Create infrastructure manager with empty spec (not needed for teardown)
	mgr := infrastructure.NewInfrastructureManager(infrastructure.InfrastructureSpec{}, t.artifactDir)

	// Teardown infrastructure
	if err := mgr.Teardown(ctx, state); err != nil {
		return errors.Join(err, ErrTeardownFailed)
	}

	// Remove from store
	if err := t.store.Delete(id); err != nil {
		return fmt.Errorf("failed to delete from store: %w", err)
	}

	return nil
}

// parseConfig converts forge config map to InfrastructureSpec
func (t *Testenv) parseConfig(config map[string]interface{}) (infrastructure.InfrastructureSpec, error) {
	spec := infrastructure.InfrastructureSpec{}

	// Parse network config
	network, ok := config["network"].(map[string]interface{})
	if !ok {
		return spec, errors.New("missing or invalid network config")
	}

	cidr, ok := network["cidr"].(string)
	if !ok || cidr == "" {
		return spec, errors.New("missing network.cidr")
	}
	spec.Network.CIDR = cidr

	bridge, ok := network["bridge"].(string)
	if !ok || bridge == "" {
		return spec, errors.New("missing network.bridge")
	}
	spec.Network.Bridge = bridge

	dhcpRange, ok := network["dhcpRange"].(string)
	if !ok || dhcpRange == "" {
		return spec, errors.New("missing network.dhcpRange")
	}
	spec.Network.DHCPRange = dhcpRange

	// Parse KIND config
	kind, ok := config["kind"].(map[string]interface{})
	if !ok {
		return spec, errors.New("missing or invalid kind config")
	}

	clusterName, ok := kind["clusterName"].(string)
	if !ok || clusterName == "" {
		return spec, errors.New("missing kind.clusterName")
	}
	spec.Kind.ClusterName = clusterName

	// Version is optional
	if version, ok := kind["version"].(string); ok {
		spec.Kind.Version = version
	}

	// Parse shaper config
	shaper, ok := config["shaper"].(map[string]interface{})
	if !ok {
		return spec, errors.New("missing or invalid shaper config")
	}

	namespace, ok := shaper["namespace"].(string)
	if !ok || namespace == "" {
		return spec, errors.New("missing shaper.namespace")
	}
	spec.Shaper.Namespace = namespace

	// API replicas is optional, default to 1
	spec.Shaper.APIReplicas = 1
	if apiReplicas, ok := shaper["apiReplicas"].(int); ok {
		spec.Shaper.APIReplicas = apiReplicas
	} else if apiReplicas, ok := shaper["apiReplicas"].(float64); ok {
		spec.Shaper.APIReplicas = int(apiReplicas)
	}

	return spec, nil
}

// LoadScenario loads a test scenario from file
// This is a helper for binaries that need to load scenarios
func LoadScenario(scenarioPath string) (*scenario.TestScenario, error) {
	loader := scenario.NewLoader(".")
	return loader.Load(scenarioPath)
}
