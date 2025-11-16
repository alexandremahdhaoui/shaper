//go:build e2e

package orchestration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e"
	"github.com/alexandremahdhaoui/shaper/pkg/vmm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLogCollector(t *testing.T) {
	tests := []struct {
		name          string
		artifactDir   string
		expectError   bool
		errorContains string
	}{
		{
			name:        "valid artifact directory",
			artifactDir: t.TempDir(),
			expectError: false,
		},
		{
			name:          "empty artifact directory",
			artifactDir:   "",
			expectError:   true,
			errorContains: "artifact directory cannot be empty",
		},
		{
			name:        "non-existent directory creates it",
			artifactDir: filepath.Join(t.TempDir(), "new-dir"),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector, err := NewLogCollector(tt.artifactDir)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, collector)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.True(t, errors.Is(err, ErrInvalidArtifactDir))
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, collector)
				assert.Equal(t, tt.artifactDir, collector.artifactDir)

				// Verify directory was created
				_, err := os.Stat(tt.artifactDir)
				assert.NoError(t, err)
			}
		})
	}
}

func TestCollectDnsmasqLogs(t *testing.T) {
	tests := []struct {
		name          string
		setupFunc     func(t *testing.T) (dnsmasqID, tempDirRoot string)
		expectError   bool
		errorContains string
		verifyFunc    func(t *testing.T, leasePath string, collector *LogCollector)
	}{
		{
			name: "successful collection with both files",
			setupFunc: func(t *testing.T) (string, string) {
				tempDir := t.TempDir()
				dnsmasqID := "test-dnsmasq-123"

				// Create lease file
				leaseFile := filepath.Join(tempDir, "dnsmasq.leases")
				err := os.WriteFile(leaseFile, []byte("1234567890 aa:bb:cc:dd:ee:ff 192.168.1.100 testvm *\n"), 0o644)
				require.NoError(t, err)

				// Create config file
				configFile := filepath.Join("/tmp", "dnsmasq-"+dnsmasqID+".conf")
				err = os.WriteFile(configFile, []byte("dhcp-range=192.168.1.10,192.168.1.250\n"), 0o644)
				require.NoError(t, err)
				t.Cleanup(func() { os.Remove(configFile) })

				return dnsmasqID, tempDir
			},
			expectError: false,
			verifyFunc: func(t *testing.T, leasePath string, collector *LogCollector) {
				// Verify lease file was copied
				data, err := os.ReadFile(leasePath)
				assert.NoError(t, err)
				assert.Contains(t, string(data), "192.168.1.100")

				// Verify config file was copied
				configPath := filepath.Join(collector.artifactDir, "logs", "dnsmasq.conf")
				data, err = os.ReadFile(configPath)
				assert.NoError(t, err)
				assert.Contains(t, string(data), "dhcp-range")
			},
		},
		{
			name: "missing lease file creates placeholder",
			setupFunc: func(t *testing.T) (string, string) {
				tempDir := t.TempDir()
				dnsmasqID := "test-dnsmasq-456"

				// Don't create lease file - it's missing

				// Create config file
				configFile := filepath.Join("/tmp", "dnsmasq-"+dnsmasqID+".conf")
				err := os.WriteFile(configFile, []byte("dhcp-range=192.168.1.10,192.168.1.250\n"), 0o644)
				require.NoError(t, err)
				t.Cleanup(func() { os.Remove(configFile) })

				return dnsmasqID, tempDir
			},
			expectError: false,
			verifyFunc: func(t *testing.T, leasePath string, collector *LogCollector) {
				// Verify placeholder was created
				data, err := os.ReadFile(leasePath)
				assert.NoError(t, err)
				assert.Contains(t, string(data), "No DHCP leases found")
			},
		},
		{
			name: "missing config file creates placeholder",
			setupFunc: func(t *testing.T) (string, string) {
				tempDir := t.TempDir()
				dnsmasqID := "test-dnsmasq-789"

				// Create lease file
				leaseFile := filepath.Join(tempDir, "dnsmasq.leases")
				err := os.WriteFile(leaseFile, []byte("lease data\n"), 0o644)
				require.NoError(t, err)

				// Don't create config file

				return dnsmasqID, tempDir
			},
			expectError: false,
			verifyFunc: func(t *testing.T, leasePath string, collector *LogCollector) {
				// Verify config placeholder was created
				configPath := filepath.Join(collector.artifactDir, "logs", "dnsmasq.conf")
				data, err := os.ReadFile(configPath)
				assert.NoError(t, err)
				assert.Contains(t, string(data), "Dnsmasq config not found")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			artifactDir := t.TempDir()
			collector, err := NewLogCollector(artifactDir)
			require.NoError(t, err)

			dnsmasqID, tempDirRoot := tt.setupFunc(t)

			ctx := context.Background()
			leasePath, err := collector.CollectDnsmasqLogs(ctx, dnsmasqID, tempDirRoot)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, leasePath)

				if tt.verifyFunc != nil {
					tt.verifyFunc(t, leasePath, collector)
				}
			}
		})
	}
}

func TestCollectShaperAPILogs(t *testing.T) {
	// Note: This test requires kubectl to be available
	// We test error handling since we can't reliably test success without a real cluster

	artifactDir := t.TempDir()
	collector, err := NewLogCollector(artifactDir)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("invalid kubeconfig produces error file", func(t *testing.T) {
		invalidKubeconfig := filepath.Join(t.TempDir(), "invalid-kubeconfig")
		err := os.WriteFile(invalidKubeconfig, []byte("invalid yaml"), 0o644)
		require.NoError(t, err)

		logFile, err := collector.CollectShaperAPILogs(ctx, invalidKubeconfig, "default")

		// Should return error but still create log file with error message
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrLogCollectionFailed))
		assert.NotEmpty(t, logFile)

		// Verify error file was created
		data, readErr := os.ReadFile(logFile)
		assert.NoError(t, readErr)
		assert.Contains(t, string(data), "Failed to collect shaper-api logs")
	})

	t.Run("context cancellation", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		invalidKubeconfig := filepath.Join(t.TempDir(), "kubeconfig")
		err := os.WriteFile(invalidKubeconfig, []byte("apiVersion: v1"), 0o644)
		require.NoError(t, err)

		_, err = collector.CollectShaperAPILogs(cancelCtx, invalidKubeconfig, "default")
		assert.Error(t, err)
	})
}

func TestCollectVMLogs(t *testing.T) {
	artifactDir := t.TempDir()
	collector, err := NewLogCollector(artifactDir)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("nil VM instance", func(t *testing.T) {
		logFile, err := collector.CollectVMLogs(ctx, nil)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrLogCollectionFailed))
		assert.Empty(t, logFile)
		assert.Contains(t, err.Error(), "VM instance is nil")
	})

	t.Run("VM without console creates placeholder", func(t *testing.T) {
		vm := &VMInstance{
			Spec: VMSpec{
				Name: "test-vm-noconsole",
			},
			State: VMStateRunning,
		}

		logFile, err := collector.CollectVMLogs(ctx, vm)

		// Should not error - console logs are optional
		assert.NoError(t, err)
		assert.NotEmpty(t, logFile)

		// Verify placeholder was created
		data, readErr := os.ReadFile(logFile)
		assert.NoError(t, readErr)
		assert.Contains(t, string(data), "VM console not available")
	})
}

func TestCollectKindLogs(t *testing.T) {
	// Note: This test requires kind to be available
	// We test error handling since we can't reliably test success without a real cluster

	artifactDir := t.TempDir()
	collector, err := NewLogCollector(artifactDir)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("non-existent cluster produces error", func(t *testing.T) {
		logsDir, err := collector.CollectKindLogs(ctx, "non-existent-cluster-12345")

		// Should return error but still create directory with error file
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrLogCollectionFailed))
		assert.NotEmpty(t, logsDir)

		// Verify error file was created
		errorFile := filepath.Join(logsDir, "export-error.txt")
		data, readErr := os.ReadFile(errorFile)
		assert.NoError(t, readErr)
		assert.Contains(t, string(data), "Failed to export kind logs")
	})

	t.Run("context cancellation", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err = collector.CollectKindLogs(cancelCtx, "some-cluster")
		assert.Error(t, err)
	})
}

func TestCollectAll(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(t *testing.T) (*e2e.ShaperTestEnvironment, []*VMInstance)
		expectError bool
		verifyFunc  func(t *testing.T, collection *LogCollection, err error)
	}{
		{
			name: "nil infrastructure",
			setupFunc: func(t *testing.T) (*e2e.ShaperTestEnvironment, []*VMInstance) {
				return nil, nil
			},
			expectError: true,
			verifyFunc: func(t *testing.T, collection *LogCollection, err error) {
				assert.Nil(t, collection)
				assert.Contains(t, err.Error(), "infrastructure state is nil")
			},
		},
		{
			name: "minimal infrastructure with dnsmasq only",
			setupFunc: func(t *testing.T) (*e2e.ShaperTestEnvironment, []*VMInstance) {
				tempDir := t.TempDir()
				dnsmasqID := "test-dnsmasq-collect-all"

				// Create lease file
				leaseFile := filepath.Join(tempDir, "dnsmasq.leases")
				err := os.WriteFile(leaseFile, []byte("lease data\n"), 0o644)
				require.NoError(t, err)

				// Create config file
				configFile := filepath.Join("/tmp", "dnsmasq-"+dnsmasqID+".conf")
				err = os.WriteFile(configFile, []byte("config data\n"), 0o644)
				require.NoError(t, err)
				t.Cleanup(func() { os.Remove(configFile) })

				infra := &e2e.ShaperTestEnvironment{
					ID:          "test-env",
					DnsmasqID:   dnsmasqID,
					TempDirRoot: tempDir,
				}

				return infra, nil
			},
			expectError: false,
			verifyFunc: func(t *testing.T, collection *LogCollection, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, collection)
				assert.NotEmpty(t, collection.DnsmasqLeases)
				assert.NotEmpty(t, collection.DnsmasqConfig)
			},
		},
		{
			name: "full infrastructure with VM",
			setupFunc: func(t *testing.T) (*e2e.ShaperTestEnvironment, []*VMInstance) {
				tempDir := t.TempDir()
				dnsmasqID := "test-dnsmasq-full"

				// Create lease file
				leaseFile := filepath.Join(tempDir, "dnsmasq.leases")
				err := os.WriteFile(leaseFile, []byte("lease data\n"), 0o644)
				require.NoError(t, err)

				// Create config file
				configFile := filepath.Join("/tmp", "dnsmasq-"+dnsmasqID+".conf")
				err = os.WriteFile(configFile, []byte("config data\n"), 0o644)
				require.NoError(t, err)
				t.Cleanup(func() { os.Remove(configFile) })

				infra := &e2e.ShaperTestEnvironment{
					ID:              "test-env-full",
					DnsmasqID:       dnsmasqID,
					TempDirRoot:     tempDir,
					Kubeconfig:      filepath.Join(t.TempDir(), "invalid-kubeconfig"),
					KindCluster:     "non-existent-cluster",
					ShaperNamespace: "default",
				}

				vms := []*VMInstance{
					{
						Spec: VMSpec{
							Name: "test-vm-1",
						},
						Metadata: &vmm.VMMetadata{
							Name: "test-vm-1",
						},
						State: VMStateRunning,
					},
				}

				return infra, vms
			},
			expectError: true, // Will have errors from kubectl and kind
			verifyFunc: func(t *testing.T, collection *LogCollection, err error) {
				assert.Error(t, err)
				assert.NotNil(t, collection)

				// Should have attempted all collections
				assert.NotEmpty(t, collection.DnsmasqLeases)
				assert.NotEmpty(t, collection.DnsmasqConfig)
				assert.NotEmpty(t, collection.ShaperAPILogs)
				assert.NotEmpty(t, collection.KindClusterLogs)
				assert.Len(t, collection.VMLogs, 1)

				// Error should be a joined error
				assert.Contains(t, err.Error(), "failed to collect")
			},
		},
		{
			name: "infrastructure with nil VM in list",
			setupFunc: func(t *testing.T) (*e2e.ShaperTestEnvironment, []*VMInstance) {
				tempDir := t.TempDir()
				dnsmasqID := "test-dnsmasq-nil-vm"

				// Create minimal setup
				leaseFile := filepath.Join(tempDir, "dnsmasq.leases")
				err := os.WriteFile(leaseFile, []byte("lease\n"), 0o644)
				require.NoError(t, err)

				configFile := filepath.Join("/tmp", "dnsmasq-"+dnsmasqID+".conf")
				err = os.WriteFile(configFile, []byte("config\n"), 0o644)
				require.NoError(t, err)
				t.Cleanup(func() { os.Remove(configFile) })

				infra := &e2e.ShaperTestEnvironment{
					ID:          "test-env-nil-vm",
					DnsmasqID:   dnsmasqID,
					TempDirRoot: tempDir,
				}

				vms := []*VMInstance{
					nil, // Nil VM should be skipped
					{
						Spec: VMSpec{
							Name: "test-vm-valid",
						},
						State: VMStateRunning,
					},
				}

				return infra, vms
			},
			expectError: false,
			verifyFunc: func(t *testing.T, collection *LogCollection, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, collection)
				// Should only have logs for the valid VM
				assert.Len(t, collection.VMLogs, 1)
				assert.Contains(t, collection.VMLogs, "test-vm-valid")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			artifactDir := t.TempDir()
			collector, err := NewLogCollector(artifactDir)
			require.NoError(t, err)

			infra, vms := tt.setupFunc(t)

			ctx := context.Background()
			collection, err := collector.CollectAll(ctx, infra, vms)

			if tt.verifyFunc != nil {
				tt.verifyFunc(t, collection, err)
			} else if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, collection)
			}

			// Verify logs directory was created
			logsDir := filepath.Join(artifactDir, "logs")
			_, statErr := os.Stat(logsDir)
			if !tt.expectError || collection != nil {
				assert.NoError(t, statErr)
			}
		})
	}
}

func TestCopyFile(t *testing.T) {
	tests := []struct {
		name          string
		setupFunc     func(t *testing.T) (src, dst string)
		expectError   bool
		errorContains string
	}{
		{
			name: "successful copy",
			setupFunc: func(t *testing.T) (string, string) {
				src := filepath.Join(t.TempDir(), "source.txt")
				dst := filepath.Join(t.TempDir(), "dest.txt")

				err := os.WriteFile(src, []byte("test content"), 0o644)
				require.NoError(t, err)

				return src, dst
			},
			expectError: false,
		},
		{
			name: "non-existent source",
			setupFunc: func(t *testing.T) (string, string) {
				src := filepath.Join(t.TempDir(), "non-existent.txt")
				dst := filepath.Join(t.TempDir(), "dest.txt")
				return src, dst
			},
			expectError:   true,
			errorContains: "no such file or directory",
		},
		{
			name: "creates destination directory",
			setupFunc: func(t *testing.T) (string, string) {
				tmpDir := t.TempDir()
				src := filepath.Join(tmpDir, "source.txt")
				dst := filepath.Join(tmpDir, "subdir", "nested", "dest.txt")

				err := os.WriteFile(src, []byte("test"), 0o644)
				require.NoError(t, err)

				return src, dst
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, dst := tt.setupFunc(t)

			err := copyFile(src, dst)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)

				// Verify file was copied
				srcData, _ := os.ReadFile(src)
				dstData, _ := os.ReadFile(dst)
				assert.Equal(t, srcData, dstData)
			}
		})
	}
}
