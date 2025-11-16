//go:build e2e

package orchestration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alexandremahdhaoui/shaper/pkg/execcontext"
	"github.com/alexandremahdhaoui/shaper/pkg/vmm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockVMM is a mock implementation of VMM for testing
type mockVMM struct {
	createVMFunc  func(cfg vmm.VMConfig) (*vmm.VMMetadata, error)
	destroyVMFunc func(ctx execcontext.Context, vmName string) error
}

func (m *mockVMM) CreateVM(cfg vmm.VMConfig) (*vmm.VMMetadata, error) {
	if m.createVMFunc != nil {
		return m.createVMFunc(cfg)
	}
	return &vmm.VMMetadata{
		Name: cfg.Name,
		IP:   "192.168.1.100",
	}, nil
}

func (m *mockVMM) DestroyVM(ctx execcontext.Context, vmName string) error {
	if m.destroyVMFunc != nil {
		return m.destroyVMFunc(ctx, vmName)
	}
	return nil
}

func (m *mockVMM) Close() error {
	return nil
}

func TestNewVMOrchestrator(t *testing.T) {
	mock := &mockVMM{}
	orch := NewVMOrchestrator(mock, "test-network")

	assert.NotNil(t, orch)
	assert.Equal(t, "test-network", orch.network)
	assert.NotNil(t, orch.events)
	assert.Equal(t, 0, len(orch.events))
}

func TestProvisionVM(t *testing.T) {
	tests := []struct {
		name          string
		spec          VMSpec
		mockCreateVM  func(cfg vmm.VMConfig) (*vmm.VMMetadata, error)
		expectError   bool
		expectedState VMState
	}{
		{
			name: "successful VM provisioning",
			spec: VMSpec{
				Name:   "test-vm",
				Memory: "2048",
				VCPUs:  2,
			},
			mockCreateVM: func(cfg vmm.VMConfig) (*vmm.VMMetadata, error) {
				return &vmm.VMMetadata{
					Name: cfg.Name,
					IP:   "192.168.1.100",
				}, nil
			},
			expectError:   false,
			expectedState: VMStateRunning,
		},
		{
			name: "VM provisioning failure",
			spec: VMSpec{
				Name:   "failing-vm",
				Memory: "2048",
				VCPUs:  2,
			},
			mockCreateVM: func(cfg vmm.VMConfig) (*vmm.VMMetadata, error) {
				return nil, errors.New("libvirt connection failed")
			},
			expectError: true,
		},
		{
			name: "VM with disk configuration",
			spec: VMSpec{
				Name:   "disk-vm",
				Memory: "4096",
				VCPUs:  4,
				Disk: &DiskSpec{
					Image: "/path/to/base.qcow2",
					Size:  "20G",
				},
			},
			mockCreateVM: func(cfg vmm.VMConfig) (*vmm.VMMetadata, error) {
				assert.Equal(t, "/path/to/base.qcow2", cfg.ImageQCOW2Path)
				assert.Equal(t, "20G", cfg.DiskSize)
				return &vmm.VMMetadata{
					Name: cfg.Name,
					IP:   "192.168.1.101",
				}, nil
			},
			expectError:   false,
			expectedState: VMStateRunning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockVMM{
				createVMFunc: tt.mockCreateVM,
			}
			orch := NewVMOrchestrator(mock, "test-network")

			ctx := context.Background()
			instance, err := orch.ProvisionVM(ctx, tt.spec)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, instance)
				assert.True(t, errors.Is(err, ErrVMProvisionFailed))
			} else {
				assert.NoError(t, err)
				require.NotNil(t, instance)
				assert.Equal(t, tt.spec.Name, instance.Spec.Name)
				assert.Equal(t, tt.expectedState, instance.State)
				assert.NotNil(t, instance.Metadata)
			}

			// Verify events were recorded
			events := orch.GetEvents()
			assert.Greater(t, len(events), 0)
		})
	}
}

func TestProvisionMultiple(t *testing.T) {
	tests := []struct {
		name         string
		specs        []VMSpec
		mockCreateVM func(cfg vmm.VMConfig) (*vmm.VMMetadata, error)
		expectError  bool
		expectedVMs  int
	}{
		{
			name: "successful parallel provisioning",
			specs: []VMSpec{
				{Name: "vm1", Memory: "2048", VCPUs: 2},
				{Name: "vm2", Memory: "2048", VCPUs: 2},
				{Name: "vm3", Memory: "2048", VCPUs: 2},
			},
			mockCreateVM: func(cfg vmm.VMConfig) (*vmm.VMMetadata, error) {
				// Simulate some work
				time.Sleep(10 * time.Millisecond)
				return &vmm.VMMetadata{
					Name: cfg.Name,
					IP:   "192.168.1.100",
				}, nil
			},
			expectError: false,
			expectedVMs: 3,
		},
		{
			name: "partial failure triggers cleanup",
			specs: []VMSpec{
				{Name: "vm1", Memory: "2048", VCPUs: 2},
				{Name: "vm-fail", Memory: "2048", VCPUs: 2},
				{Name: "vm3", Memory: "2048", VCPUs: 2},
			},
			mockCreateVM: func(cfg vmm.VMConfig) (*vmm.VMMetadata, error) {
				if cfg.Name == "vm-fail" {
					return nil, errors.New("provisioning failed")
				}
				return &vmm.VMMetadata{
					Name: cfg.Name,
					IP:   "192.168.1.100",
				}, nil
			},
			expectError: true,
			expectedVMs: 0, // All should be cleaned up on error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			destroyCalled := 0
			mock := &mockVMM{
				createVMFunc: tt.mockCreateVM,
				destroyVMFunc: func(ctx execcontext.Context, vmName string) error {
					destroyCalled++
					return nil
				},
			}
			orch := NewVMOrchestrator(mock, "test-network")

			ctx := context.Background()
			instances, err := orch.ProvisionMultiple(ctx, tt.specs)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, instances)
				// Verify cleanup was attempted
				assert.Greater(t, destroyCalled, 0, "DestroyVM should be called for cleanup")
			} else {
				assert.NoError(t, err)
				require.NotNil(t, instances)
				assert.Equal(t, tt.expectedVMs, len(instances))
				for _, inst := range instances {
					assert.NotNil(t, inst)
					assert.Equal(t, VMStateRunning, inst.State)
				}
			}
		})
	}
}

func TestDestroyVM(t *testing.T) {
	tests := []struct {
		name          string
		instance      *VMInstance
		mockDestroyVM func(ctx execcontext.Context, vmName string) error
		expectError   bool
	}{
		{
			name: "successful VM destruction",
			instance: &VMInstance{
				Spec:  VMSpec{Name: "test-vm"},
				State: VMStateRunning,
			},
			mockDestroyVM: func(ctx execcontext.Context, vmName string) error {
				return nil
			},
			expectError: false,
		},
		{
			name:        "nil instance",
			instance:    nil,
			expectError: true,
		},
		{
			name: "VMM destruction failure",
			instance: &VMInstance{
				Spec:  VMSpec{Name: "failing-vm"},
				State: VMStateRunning,
			},
			mockDestroyVM: func(ctx execcontext.Context, vmName string) error {
				return errors.New("failed to destroy domain")
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockVMM{
				destroyVMFunc: tt.mockDestroyVM,
			}
			orch := NewVMOrchestrator(mock, "test-network")

			ctx := context.Background()
			err := orch.DestroyVM(ctx, tt.instance)

			if tt.expectError {
				assert.Error(t, err)
				if tt.instance == nil {
					assert.True(t, errors.Is(err, ErrVMNotFound))
				} else {
					assert.True(t, errors.Is(err, ErrVMDestroyFailed))
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, VMStateDestroyed, tt.instance.State)
			}
		})
	}
}

func TestDestroyAll(t *testing.T) {
	tests := []struct {
		name          string
		instances     []*VMInstance
		mockDestroyVM func(ctx execcontext.Context, vmName string) error
		expectError   bool
	}{
		{
			name: "destroy all VMs successfully",
			instances: []*VMInstance{
				{Spec: VMSpec{Name: "vm1"}, State: VMStateRunning},
				{Spec: VMSpec{Name: "vm2"}, State: VMStateRunning},
				{Spec: VMSpec{Name: "vm3"}, State: VMStateRunning},
			},
			mockDestroyVM: func(ctx execcontext.Context, vmName string) error {
				return nil
			},
			expectError: false,
		},
		{
			name: "partial destruction failure returns aggregated errors",
			instances: []*VMInstance{
				{Spec: VMSpec{Name: "vm1"}, State: VMStateRunning},
				{Spec: VMSpec{Name: "vm-fail"}, State: VMStateRunning},
				{Spec: VMSpec{Name: "vm3"}, State: VMStateRunning},
			},
			mockDestroyVM: func(ctx execcontext.Context, vmName string) error {
				if vmName == "vm-fail" {
					return errors.New("destruction failed")
				}
				return nil
			},
			expectError: true,
		},
		{
			name: "nil instances are skipped",
			instances: []*VMInstance{
				{Spec: VMSpec{Name: "vm1"}, State: VMStateRunning},
				nil,
				{Spec: VMSpec{Name: "vm3"}, State: VMStateRunning},
			},
			mockDestroyVM: func(ctx execcontext.Context, vmName string) error {
				return nil
			},
			expectError: false,
		},
		{
			name:        "empty instances list",
			instances:   []*VMInstance{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockVMM{
				destroyVMFunc: tt.mockDestroyVM,
			}
			orch := NewVMOrchestrator(mock, "test-network")

			ctx := context.Background()
			err := orch.DestroyAll(ctx, tt.instances)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRecordEvent(t *testing.T) {
	mock := &mockVMM{}
	orch := NewVMOrchestrator(mock, "test-network")

	orch.RecordEvent("test-vm", "provision_start", "Starting VM")
	orch.RecordEvent("test-vm", "provision_success", "VM created")

	events := orch.GetEvents()
	assert.Equal(t, 2, len(events))
	assert.Equal(t, "test-vm", events[0].VMName)
	assert.Equal(t, "provision_start", events[0].EventType)
	assert.Equal(t, "Starting VM", events[0].Details)
	assert.Equal(t, "provision_success", events[1].EventType)
	assert.False(t, events[0].Timestamp.IsZero())
}

func TestSpecToVMMConfig(t *testing.T) {
	tests := []struct {
		name     string
		spec     VMSpec
		network  string
		validate func(*testing.T, vmm.VMConfig)
	}{
		{
			name: "basic VM spec",
			spec: VMSpec{
				Name:   "test-vm",
				Memory: "2048",
				VCPUs:  2,
			},
			network: "test-network",
			validate: func(t *testing.T, cfg vmm.VMConfig) {
				assert.Equal(t, "test-vm", cfg.Name)
				assert.Equal(t, "test-network", cfg.Network)
				assert.Equal(t, uint(2048), cfg.MemoryMB)
				assert.Equal(t, uint(2), cfg.VCPUs)
			},
		},
		{
			name: "VM with disk",
			spec: VMSpec{
				Name:   "disk-vm",
				Memory: "4096",
				VCPUs:  4,
				Disk: &DiskSpec{
					Image: "/path/to/image.qcow2",
					Size:  "40G",
				},
			},
			network: "test-network",
			validate: func(t *testing.T, cfg vmm.VMConfig) {
				assert.Equal(t, "disk-vm", cfg.Name)
				assert.Equal(t, uint(4096), cfg.MemoryMB)
				assert.Equal(t, uint(4), cfg.VCPUs)
				assert.Equal(t, "/path/to/image.qcow2", cfg.ImageQCOW2Path)
				assert.Equal(t, "40G", cfg.DiskSize)
			},
		},
		{
			name: "VM without memory/vcpu uses defaults",
			spec: VMSpec{
				Name: "minimal-vm",
			},
			network: "test-network",
			validate: func(t *testing.T, cfg vmm.VMConfig) {
				assert.Equal(t, "minimal-vm", cfg.Name)
				assert.Equal(t, "test-network", cfg.Network)
				// VMM will use its defaults for MemoryMB and VCPUs
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockVMM{}
			orch := NewVMOrchestrator(mock, tt.network)
			cfg := orch.specToVMMConfig(tt.spec)
			tt.validate(t, cfg)
		})
	}
}

func TestVMStates(t *testing.T) {
	// Test VM state constants
	assert.Equal(t, VMState("created"), VMStateCreated)
	assert.Equal(t, VMState("running"), VMStateRunning)
	assert.Equal(t, VMState("stopped"), VMStateStopped)
	assert.Equal(t, VMState("destroyed"), VMStateDestroyed)
}

// TestVMMImplementsInterface verifies that *vmm.VMM implements VMMInterface
// This is a compile-time check to ensure our interface matches the actual VMM
func TestVMMImplementsInterface(t *testing.T) {
	// This test will fail to compile if *vmm.VMM doesn't implement VMMInterface
	var _ VMMInterface = (*vmm.VMM)(nil)
	// If this compiles, the test passes
	t.Log("*vmm.VMM implements VMMInterface")
}
