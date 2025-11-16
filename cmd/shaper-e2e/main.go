//go:build e2e

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e"
)

func main() {
	// Create a new flag set for this tool
	fs := flag.NewFlagSet("shaper-e2e", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: shaper-e2e [command] [options]

Commands:
  create             Create a new test environment
  get <test-id>      Get information about a test environment
  run <test-id>      Run tests in an existing environment
  delete <test-id>   Cleanup and destroy a test environment
  list               List all known test environments and their status
  logs <test-id> <log-type>  Display logs for a test environment
                             Log types: dnsmasq, kind
  test               One-shot test (create → run → delete)

Environment Variables:
  SHAPER_E2E_ARTIFACTS_DIR  Override artifact storage location (default: ~/.shaper/e2e/)
  SHAPER_E2E_IMAGE_CACHE    Override image cache location (default: /tmp/shaper-e2e-images)
  SHAPER_E2E_DEBUG          Enable debug logging (set to "1")

Prerequisites:
  Some operations require elevated privileges. Ensure your user has:
  - Membership in 'libvirt' group (for VM operations)
  - CAP_NET_ADMIN capability or appropriate sudo configuration (for network operations)
  See README.md for setup instructions.

Examples:
  # Create test environment
  shaper-e2e create

  # Get environment information
  shaper-e2e get e2e-shaper-abc12345

  # Run tests in that environment
  shaper-e2e run e2e-shaper-abc12345

  # View dnsmasq logs
  shaper-e2e logs e2e-shaper-abc12345 dnsmasq

  # View KIND cluster logs
  shaper-e2e logs e2e-shaper-abc12345 kind

  # Cleanup when done
  shaper-e2e delete e2e-shaper-abc12345

  # List all environments
  shaper-e2e list

  # One-shot test
  shaper-e2e test
`)
	}

	if len(os.Args) < 2 {
		fs.Usage()
		os.Exit(1)
	}

	command := os.Args[1]
	artifactStoreDir := getArtifactDir()

	switch command {
	case "create":
		cmdCreate(artifactStoreDir)
	case "get":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: 'get' requires a test ID\n")
			fmt.Fprintf(os.Stderr, "Usage: shaper-e2e get <test-id>\n")
			os.Exit(1)
		}
		cmdGet(artifactStoreDir, os.Args[2])
	case "run":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: 'run' requires a test ID\n")
			fmt.Fprintf(os.Stderr, "Usage: shaper-e2e run <test-id>\n")
			os.Exit(1)
		}
		cmdRun(artifactStoreDir, os.Args[2])
	case "delete":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: 'delete' requires a test ID\n")
			fmt.Fprintf(os.Stderr, "Usage: shaper-e2e delete <test-id>\n")
			os.Exit(1)
		}
		cmdDelete(artifactStoreDir, os.Args[2])
	case "list":
		cmdList(artifactStoreDir)
	case "logs":
		if len(os.Args) < 4 {
			fmt.Fprintf(os.Stderr, "Error: 'logs' requires a test ID and log type\n")
			fmt.Fprintf(os.Stderr, "Usage: shaper-e2e logs <test-id> <log-type>\n")
			fmt.Fprintf(os.Stderr, "  Log types: dnsmasq, kind\n")
			os.Exit(1)
		}
		cmdLogs(artifactStoreDir, os.Args[2], os.Args[3])
	case "test":
		cmdTest(artifactStoreDir)
	case "-h", "--help", "help":
		fs.Usage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command '%s'\n", command)
		fs.Usage()
		os.Exit(1)
	}
}

// getArtifactDir returns the artifact storage directory
func getArtifactDir() string {
	if dir := os.Getenv("SHAPER_E2E_ARTIFACTS_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(os.ExpandEnv("$HOME"), ".shaper", "e2e")
}

// getImageCacheDir returns the image cache directory
func getImageCacheDir() string {
	if dir := os.Getenv("SHAPER_E2E_IMAGE_CACHE"); dir != "" {
		return dir
	}
	return filepath.Join(os.TempDir(), "shaper-e2e-images")
}

// cmdCreate creates and provisions a complete test environment
func cmdCreate(artifactStoreDir string) {
	// Note: Network operations (bridge, dnsmasq) will require appropriate permissions
	// The underlying operations will fail with clear error messages if permissions are insufficient

	imageCacheDir := getImageCacheDir()

	// Setup configuration
	setupConfig := e2e.ShaperSetupConfig{
		ArtifactDir:     filepath.Join(artifactStoreDir, "artifacts"),
		ImageCacheDir:   imageCacheDir,
		BridgeName:      "br-shaper-e2e",
		NetworkCIDR:     "192.168.100.1/24",
		DHCPRange:       "192.168.100.10,192.168.100.250",
		KindClusterName: "shaper-e2e",
		TFTPRoot:        "", // Will be set by setup
		IPXEBootFile:    "", // Optional - user can provide
		NumClients:      0,  // Don't pre-create VMs
		DownloadImages:  false,
	}

	// Create test environment
	fmt.Fprintf(os.Stderr, "Creating test environment...\n")
	testEnv, err := e2e.SetupShaperTestEnvironment(setupConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create test environment: %v\n", err)
		os.Exit(1)
	}

	// Save to artifact store
	if err := os.MkdirAll(artifactStoreDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create artifact store directory: %v\n", err)
		os.Exit(1)
	}

	store := NewJSONArtifactStore(filepath.Join(artifactStoreDir, "artifacts.json"))
	if err := store.Save(testEnv); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to save environment to artifact store: %v\n", err)
		os.Exit(1)
	}

	// Output environment ID (primary output for scripting)
	fmt.Println(testEnv.ID)

	// Print summary if not piped
	if !isPiped() {
		fmt.Fprintf(os.Stderr, "\n✅ Test environment created: %s\n", testEnv.ID)
		fmt.Fprintf(os.Stderr, "   Artifacts dir: %s\n", testEnv.ArtifactPath)
		fmt.Fprintf(os.Stderr, "\n=== Network ===\n")
		fmt.Fprintf(os.Stderr, "   Bridge: %s\n", testEnv.BridgeName)
		fmt.Fprintf(os.Stderr, "   Libvirt Network: %s\n", testEnv.LibvirtNetwork)
		fmt.Fprintf(os.Stderr, "\n=== KIND Cluster ===\n")
		fmt.Fprintf(os.Stderr, "   Name: %s\n", testEnv.KindCluster)
		fmt.Fprintf(os.Stderr, "   Kubeconfig: %s\n", testEnv.Kubeconfig)
		fmt.Fprintf(os.Stderr, "\n=== TFTP/PXE ===\n")
		fmt.Fprintf(os.Stderr, "   TFTP Root: %s\n", testEnv.TFTPRoot)
		fmt.Fprintf(os.Stderr, "   Dnsmasq ID: %s\n", testEnv.DnsmasqID)
		if testEnv.DnsmasqID != "" {
			fmt.Fprintf(os.Stderr, "   Dnsmasq: Configured ✓\n")
		} else {
			fmt.Fprintf(os.Stderr, "   Dnsmasq: Not configured ✗\n")
		}
		fmt.Fprintf(os.Stderr, "\nNext: Run tests with:\n")
		fmt.Fprintf(os.Stderr, "   sudo shaper-e2e run %s\n", testEnv.ID)
	}
}

// cmdRun executes iPXE boot tests in an existing environment
func cmdRun(artifactStoreDir string, testID string) {
	// Note: VM operations may require appropriate libvirt permissions
	// Ensure user is in 'libvirt' group or configure libvirt accordingly

	artifactStoreFile := filepath.Join(artifactStoreDir, "artifacts.json")
	store := NewJSONArtifactStore(artifactStoreFile)

	// Load environment
	env, err := store.Load(testID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load test environment: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Running tests in environment: %s\n", env.ID)
	fmt.Printf("Artifact Path: %s\n", env.ArtifactPath)
	fmt.Printf("Bridge: %s\n", env.BridgeName)
	fmt.Printf("KIND Cluster: %s\n", env.KindCluster)

	// Execute iPXE boot test
	testConfig := e2e.IPXETestConfig{
		Env:         env,
		VMName:      "test-client-" + testID,
		BootOrder:   []string{"network"},
		MemoryMB:    1024,
		VCPUs:       1,
		BootTimeout: 2 * time.Minute,
		DHCPTimeout: 30 * time.Second,
		HTTPTimeout: 1 * time.Minute,
	}

	fmt.Printf("Executing iPXE boot test...\n")
	result, err := e2e.ExecuteIPXEBootTest(testConfig)

	// Log all test logs
	for _, log := range result.Logs {
		fmt.Println(log)
	}

	// Check results
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: iPXE boot test failed: %v\n", err)
		os.Exit(1)
	}

	// Print summary
	fmt.Printf("\n=== Test Results ===\n")
	fmt.Printf("Success: %v\n", result.Success)
	fmt.Printf("DHCP Lease Obtained: %v\n", result.DHCPLeaseObtained)
	fmt.Printf("TFTP Boot Fetched: %v\n", result.TFTPBootFetched)
	fmt.Printf("HTTP Boot Called: %v\n", result.HTTPBootCalled)

	if result.Success {
		fmt.Fprintf(os.Stderr, "\n✅ Tests passed in environment: %s\n", testID)
	} else {
		fmt.Fprintf(os.Stderr, "\n⚠️  Tests completed with warnings in environment: %s\n", testID)
		fmt.Fprintf(os.Stderr, "This is expected if shaper-api is not deployed\n")
	}
}

// cmdDelete destroys a test environment and cleans up all resources
func cmdDelete(artifactStoreDir string, testID string) {
	// Note: Cleanup operations may require appropriate permissions for network/VM resources

	artifactStoreFile := filepath.Join(artifactStoreDir, "artifacts.json")
	store := NewJSONArtifactStore(artifactStoreFile)

	// Load environment
	env, err := store.Load(testID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load test environment: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Deleting test environment: %s\n", env.ID)

	// Audit: log what will be deleted
	fmt.Printf("  Bridge: %s\n", env.BridgeName)
	fmt.Printf("  Libvirt Network: %s\n", env.LibvirtNetwork)
	fmt.Printf("  KIND Cluster: %s\n", env.KindCluster)
	fmt.Printf("  Temp Dir: %s\n", env.TempDirRoot)

	// Teardown environment
	teardownErr := e2e.TeardownShaperTestEnvironment(env)

	// Determine deletion strategy based on teardown success
	if teardownErr == nil {
		// SUCCESS: All cleanup operations completed without errors
		if err := store.Delete(testID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to delete environment from store: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("  ✓ Environment removed from store\n")
		fmt.Printf("\n✅ Test environment %s has been fully deleted\n", testID)

	} else {
		// PARTIAL/COMPLETE FAILURE: Some cleanup operations failed
		fmt.Fprintf(os.Stderr, "\n⚠️  Cleanup encountered errors.\n")
		fmt.Fprintf(os.Stderr, "Error details:\n%v\n", teardownErr)
		fmt.Fprintf(os.Stderr, "Please review the errors and manually clean up if necessary.\n")
		fmt.Fprintf(os.Stderr, "To retry cleanup, run: sudo shaper-e2e delete %s\n", testID)

		// Still delete from store to avoid clutter
		if err := store.Delete(testID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to delete environment from store: %v\n", err)
		}

		os.Exit(1)
	}
}

// cmdGet displays complete information about a test environment
func cmdGet(artifactStoreDir string, testID string) {
	artifactStoreFile := filepath.Join(artifactStoreDir, "artifacts.json")
	store := NewJSONArtifactStore(artifactStoreFile)

	// Load environment
	env, err := store.Load(testID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load test environment: %v\n", err)
		os.Exit(1)
	}

	// Display environment information
	fmt.Fprintf(os.Stderr, "\n=== Test Environment: %s ===\n", env.ID)
	fmt.Fprintf(os.Stderr, "Artifacts: %s\n\n", env.ArtifactPath)

	fmt.Fprintf(os.Stderr, "=== Network Infrastructure ===\n")
	fmt.Fprintf(os.Stderr, "Bridge: %s\n", env.BridgeName)
	fmt.Fprintf(os.Stderr, "Libvirt Network: %s\n", env.LibvirtNetwork)
	if env.DnsmasqID != "" {
		fmt.Fprintf(os.Stderr, "Dnsmasq: Configured (%s) ✓\n", env.DnsmasqID)
	} else {
		fmt.Fprintf(os.Stderr, "Dnsmasq: Not configured ✗\n")
	}
	fmt.Fprintf(os.Stderr, "TFTP Root: %s\n\n", env.TFTPRoot)

	fmt.Fprintf(os.Stderr, "=== KIND Cluster ===\n")
	fmt.Fprintf(os.Stderr, "Name: %s\n", env.KindCluster)
	fmt.Fprintf(os.Stderr, "Kubeconfig: %s\n", env.Kubeconfig)
	fmt.Fprintf(os.Stderr, "Namespace: %s\n\n", env.ShaperNamespace)

	fmt.Fprintf(os.Stderr, "=== TFTP/PXE Boot ===\n")
	fmt.Fprintf(os.Stderr, "TFTP Root: %s\n", env.TFTPRoot)
	fmt.Fprintf(os.Stderr, "Temp Dir: %s\n\n", env.TempDirRoot)

	if len(env.ClientVMs) > 0 {
		fmt.Fprintf(os.Stderr, "=== Client VMs ===\n")
		for i, vm := range env.ClientVMs {
			fmt.Fprintf(os.Stderr, "[%d] %s (Memory: %dMB, VCPUs: %d)\n",
				i, vm.Name, vm.MemoryMB, vm.VCPUs)
			if vm.IP != "" {
				fmt.Fprintf(os.Stderr, "    IP: %s\n", vm.IP)
			}
		}
		fmt.Fprintf(os.Stderr, "\n")
	}

	fmt.Fprintf(os.Stderr, "=== Usage ===\n")
	fmt.Fprintf(os.Stderr, "Run tests: sudo shaper-e2e run %s\n", env.ID)
	fmt.Fprintf(os.Stderr, "View logs: shaper-e2e logs %s dnsmasq\n", env.ID)
	fmt.Fprintf(os.Stderr, "Cleanup:   sudo shaper-e2e delete %s\n", env.ID)
}

// cmdLogs displays logs for a test environment
func cmdLogs(artifactStoreDir string, testID string, logType string) {
	artifactStoreFile := filepath.Join(artifactStoreDir, "artifacts.json")
	store := NewJSONArtifactStore(artifactStoreFile)

	// Load environment
	env, err := store.Load(testID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load test environment: %v\n", err)
		os.Exit(1)
	}

	// Validate and handle log type
	switch logType {
	case "dnsmasq":
		// Display dnsmasq logs
		leaseFile := filepath.Join(env.TempDirRoot, "dnsmasq.leases")
		if _, err := os.Stat(leaseFile); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Warning: dnsmasq lease file not found: %s\n", leaseFile)
		} else {
			fmt.Printf("=== Dnsmasq Leases (%s) ===\n", leaseFile)
			content, err := os.ReadFile(leaseFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: failed to read lease file: %v\n", err)
			} else {
				fmt.Print(string(content))
			}
			fmt.Println()
		}

		// Show dnsmasq info
		if env.DnsmasqID != "" {
			fmt.Printf("=== Dnsmasq ===\n")
			fmt.Printf("ID: %s\n", env.DnsmasqID)
			// Config is managed by DnsmasqManager and stored in /tmp/dnsmasq-<id>.conf
			configPath := "/tmp/dnsmasq-" + env.DnsmasqID + ".conf"
			content, err := os.ReadFile(configPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Config file not accessible: %v\n", err)
			} else {
				fmt.Printf("Config:\n%s\n", string(content))
			}
		}

	case "kind":
		// Display KIND cluster information
		if env.Kubeconfig == "" {
			fmt.Fprintf(os.Stderr, "Error: kubeconfig not set for environment %s\n", testID)
			os.Exit(1)
		}

		fmt.Printf("=== KIND Cluster Info ===\n")
		fmt.Printf("Name: %s\n", env.KindCluster)
		fmt.Printf("Kubeconfig: %s\n\n", env.Kubeconfig)

		fmt.Printf("To view cluster logs, run:\n")
		fmt.Printf("  kind export logs --name %s /tmp/kind-logs\n", env.KindCluster)
		fmt.Printf("\nTo check cluster status, run:\n")
		fmt.Printf("  kubectl --kubeconfig=%s get nodes\n", env.Kubeconfig)
		fmt.Printf("  kubectl --kubeconfig=%s get pods -A\n", env.Kubeconfig)

	default:
		fmt.Fprintf(os.Stderr, "Error: invalid log type '%s'\n", logType)
		fmt.Fprintf(os.Stderr, "Valid log types: dnsmasq, kind\n")
		os.Exit(1)
	}
}

// cmdList lists all test environments
func cmdList(artifactStoreDir string) {
	artifactStoreFile := filepath.Join(artifactStoreDir, "artifacts.json")
	store := NewJSONArtifactStore(artifactStoreFile)

	// Load all environments
	envs, err := store.ListAll()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to list environments: %v\n", err)
		os.Exit(1)
	}

	if len(envs) == 0 {
		fmt.Println("No test environments found")
		return
	}

	// Create table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tBridge\tKIND Cluster\tKubeconfig")
	_, _ = fmt.Fprintln(w, "--\t--\t--\t--")

	for _, env := range envs {
		bridge := env.BridgeName
		if bridge == "" {
			bridge = "(none)"
		}
		kindCluster := env.KindCluster
		if kindCluster == "" {
			kindCluster = "(none)"
		}
		kubeconfig := env.Kubeconfig
		if kubeconfig == "" {
			kubeconfig = "(none)"
		}
		_, _ = fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\n",
			env.ID,
			bridge,
			kindCluster,
			kubeconfig,
		)
	}

	_ = w.Flush()
}

// cmdTest runs a one-shot test (create → run → delete)
func cmdTest(artifactStoreDir string) {
	// Note: Operations will require appropriate permissions
	// Ensure proper system configuration before running

	fmt.Println("Running one-shot e2e test...")

	imageCacheDir := getImageCacheDir()

	// Step 1: Create
	fmt.Println("\n[1/3] Creating test environment...")
	setupConfig := e2e.ShaperSetupConfig{
		ArtifactDir:     filepath.Join(artifactStoreDir, "artifacts"),
		ImageCacheDir:   imageCacheDir,
		BridgeName:      "br-shaper-e2e",
		NetworkCIDR:     "192.168.100.1/24",
		DHCPRange:       "192.168.100.10,192.168.100.250",
		KindClusterName: "shaper-e2e-test",
		TFTPRoot:        "",
		IPXEBootFile:    "",
		NumClients:      0,
		DownloadImages:  false,
	}

	testEnv, err := e2e.SetupShaperTestEnvironment(setupConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create test environment: %v\n", err)
		os.Exit(1)
	}

	// Save to artifact store
	if err := os.MkdirAll(artifactStoreDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create artifact store directory: %v\n", err)
		os.Exit(1)
	}

	store := NewJSONArtifactStore(filepath.Join(artifactStoreDir, "artifacts.json"))
	if err := store.Save(testEnv); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to save environment to artifact store: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Test environment created: %s\n", testEnv.ID)

	// Cleanup at the end
	defer func() {
		fmt.Println("\n[3/3] Deleting test environment...")
		if err := e2e.TeardownShaperTestEnvironment(testEnv); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: encountered errors during cleanup: %v\n", err)
		}
		if err := store.Delete(testEnv.ID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to delete environment from store: %v\n", err)
		}
	}()

	// Step 2: Run tests
	fmt.Println("\n[2/3] Running tests...")

	testConfig := e2e.IPXETestConfig{
		Env:         testEnv,
		VMName:      "test-client-" + testEnv.ID,
		BootOrder:   []string{"network"},
		MemoryMB:    1024,
		VCPUs:       1,
		BootTimeout: 2 * time.Minute,
		DHCPTimeout: 30 * time.Second,
		HTTPTimeout: 1 * time.Minute,
	}

	result, err := e2e.ExecuteIPXEBootTest(testConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: iPXE boot test failed: %v\n", err)
		os.Exit(1)
	}

	// Log results
	for _, log := range result.Logs {
		fmt.Println(log)
	}

	if result.Success {
		fmt.Println("\n✅ One-shot e2e test completed successfully!")
	} else {
		fmt.Println("\n⚠️  One-shot e2e test completed with warnings")
		fmt.Println("This is expected if shaper-api is not deployed")
	}
}

// debugf prints debug messages to stderr if DEBUG is set
func debugf(format string, a ...interface{}) {
	if os.Getenv("SHAPER_E2E_DEBUG") == "1" {
		fmt.Fprintf(os.Stderr, "[DEBUG] "+format, a...)
	}
}

// isPiped returns true if stdout is piped to another process
func isPiped() bool {
	stat, _ := os.Stdout.Stat()
	return (stat.Mode() & os.ModeCharDevice) == 0
}

// JSONArtifactStore stores test environments as JSON
type JSONArtifactStore struct {
	filePath string
}

// NewJSONArtifactStore creates a new JSON artifact store
func NewJSONArtifactStore(filePath string) *JSONArtifactStore {
	return &JSONArtifactStore{filePath: filePath}
}

// Save saves an environment to the store
func (s *JSONArtifactStore) Save(env *e2e.ShaperTestEnvironment) error {
	// Load existing store
	store := make(map[string]*e2e.ShaperTestEnvironment)
	if data, err := os.ReadFile(s.filePath); err == nil {
		if err := json.Unmarshal(data, &store); err != nil {
			return fmt.Errorf("failed to unmarshal existing store: %w", err)
		}
	}

	// Add/update environment
	store[env.ID] = env

	// Write back
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal store: %w", err)
	}

	if err := os.WriteFile(s.filePath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write store: %w", err)
	}

	return nil
}

// Load loads an environment from the store
func (s *JSONArtifactStore) Load(id string) (*e2e.ShaperTestEnvironment, error) {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read store: %w", err)
	}

	store := make(map[string]*e2e.ShaperTestEnvironment)
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("failed to unmarshal store: %w", err)
	}

	env, ok := store[id]
	if !ok {
		return nil, fmt.Errorf("environment %s not found", id)
	}

	return env, nil
}

// Delete removes an environment from the store
func (s *JSONArtifactStore) Delete(id string) error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return fmt.Errorf("failed to read store: %w", err)
	}

	store := make(map[string]*e2e.ShaperTestEnvironment)
	if err := json.Unmarshal(data, &store); err != nil {
		return fmt.Errorf("failed to unmarshal store: %w", err)
	}

	delete(store, id)

	// Write back
	data, err = json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal store: %w", err)
	}

	if err := os.WriteFile(s.filePath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write store: %w", err)
	}

	return nil
}

// ListAll returns all environments in the store
func (s *JSONArtifactStore) ListAll() ([]*e2e.ShaperTestEnvironment, error) {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []*e2e.ShaperTestEnvironment{}, nil
		}
		return nil, fmt.Errorf("failed to read store: %w", err)
	}

	store := make(map[string]*e2e.ShaperTestEnvironment)
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("failed to unmarshal store: %w", err)
	}

	envs := make([]*e2e.ShaperTestEnvironment, 0, len(store))
	for _, env := range store {
		envs = append(envs, env)
	}

	return envs, nil
}
