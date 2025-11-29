package orchestration

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alexandremahdhaoui/shaper/pkg/cloudinit"
	"github.com/alexandremahdhaoui/shaper/pkg/execcontext"
	"github.com/alexandremahdhaoui/shaper/pkg/vmm"
	"golang.org/x/sync/errgroup"
)

var (
	// ErrVMProvisionFailed indicates VM provisioning failed
	ErrVMProvisionFailed = errors.New("VM provisioning failed")
	// ErrVMDestroyFailed indicates VM destruction failed
	ErrVMDestroyFailed = errors.New("VM destruction failed")
	// ErrVMNotFound indicates VM instance not found
	ErrVMNotFound = errors.New("VM instance not found")
)

// VMMInterface defines the interface for VM management operations
// This interface allows mocking VMM for testing
type VMMInterface interface {
	CreateVM(cfg vmm.VMConfig) (*vmm.VMMetadata, error)
	DestroyVM(ctx execcontext.Context, vmName string) error
	Close() error
}

// VMState represents the state of a VM instance
type VMState string

const (
	// VMStateCreated indicates VM is created but not yet running
	VMStateCreated VMState = "created"
	// VMStateRunning indicates VM is running
	VMStateRunning VMState = "running"
	// VMStateStopped indicates VM is stopped
	VMStateStopped VMState = "stopped"
	// VMStateDestroyed indicates VM is destroyed
	VMStateDestroyed VMState = "destroyed"
)

// VMSpec defines the specification for a test VM
// This matches the architecture.md VMSpec definition
type VMSpec struct {
	Name       string
	UUID       string
	MACAddress string
	Memory     string // e.g., "2048" for 2048MB
	VCPUs      int
	BootOrder  []string // e.g., ["network", "hd"]
	Disk       *DiskSpec
	Labels     map[string]string
}

// DiskSpec defines VM disk configuration
type DiskSpec struct {
	Image string // Path to base image
	Size  string // Disk size (e.g., "20G")
}

// VMInstance represents a provisioned VM with its metadata and state
type VMInstance struct {
	Spec     VMSpec
	Metadata *vmm.VMMetadata
	State    VMState
}

// TestEvent represents a VM lifecycle event for timeline tracking
type TestEvent struct {
	Timestamp time.Time
	VMName    string
	EventType string
	Details   string
}

// VMOrchestrator manages VM lifecycle for E2E tests
type VMOrchestrator struct {
	vmm     VMMInterface
	network string // Libvirt network name
	events  []TestEvent
}

// NewVMOrchestrator creates a new VM orchestrator
func NewVMOrchestrator(vmmConn VMMInterface, network string) *VMOrchestrator {
	return &VMOrchestrator{
		vmm:     vmmConn,
		network: network,
		events:  make([]TestEvent, 0),
	}
}

// ProvisionVM creates and starts a single VM from its specification
func (o *VMOrchestrator) ProvisionVM(ctx context.Context, spec VMSpec) (*VMInstance, error) {
	o.RecordEvent(spec.Name, "provision_start", "Starting VM provisioning")

	// Convert VMSpec to vmm.VMConfig
	cfg := o.specToVMMConfig(spec)

	// Create VM using VMM
	metadata, err := o.vmm.CreateVM(cfg)
	if err != nil {
		o.RecordEvent(spec.Name, "provision_failed", fmt.Sprintf("Failed to create VM: %v", err))
		return nil, fmt.Errorf("%w: %s: %v", ErrVMProvisionFailed, spec.Name, err)
	}

	instance := &VMInstance{
		Spec:     spec,
		Metadata: metadata,
		State:    VMStateRunning, // VMM.CreateVM starts the VM
	}

	o.RecordEvent(spec.Name, "provision_success", fmt.Sprintf("VM provisioned with IP: %s", metadata.IP))
	return instance, nil
}

// ProvisionMultiple creates multiple VMs in parallel using errgroup
// Implements the parallel execution pattern from NEW-TASKS.md
// On ANY error: cancels context, waits for all goroutines, then cleans up ALL VMs
func (o *VMOrchestrator) ProvisionMultiple(ctx context.Context, specs []VMSpec) ([]*VMInstance, error) {
	instances := make([]*VMInstance, len(specs))
	g, ctx := errgroup.WithContext(ctx)

	for i, spec := range specs {
		i, spec := i, spec // Capture loop vars
		g.Go(func() error {
			instance, err := o.ProvisionVM(ctx, spec)
			if err != nil {
				return fmt.Errorf("provisioning VM %s: %w", spec.Name, err)
			}
			instances[i] = instance
			return nil
		})
	}

	// Wait for all VMs or first error
	if err := g.Wait(); err != nil {
		// On error: Attempt cleanup of any created VMs
		// Use background context to ensure cleanup completes even if ctx is cancelled
		o.RecordEvent("", "provision_multiple_failed", fmt.Sprintf("Parallel provisioning failed: %v", err))
		_ = o.DestroyAll(context.Background(), instances)
		return nil, err
	}

	o.RecordEvent("", "provision_multiple_success", fmt.Sprintf("Successfully provisioned %d VMs", len(instances)))
	return instances, nil
}

// DestroyVM destroys a single VM instance
func (o *VMOrchestrator) DestroyVM(ctx context.Context, instance *VMInstance) error {
	if instance == nil {
		return ErrVMNotFound
	}

	o.RecordEvent(instance.Spec.Name, "destroy_start", "Starting VM destruction")

	// Create execcontext for VMM operations
	execCtx := execcontext.New(nil, nil)
	err := o.vmm.DestroyVM(execCtx, instance.Spec.Name)
	if err != nil {
		o.RecordEvent(instance.Spec.Name, "destroy_failed", fmt.Sprintf("Failed to destroy VM: %v", err))
		return fmt.Errorf("%w: %s: %v", ErrVMDestroyFailed, instance.Spec.Name, err)
	}

	instance.State = VMStateDestroyed
	o.RecordEvent(instance.Spec.Name, "destroy_success", "VM destroyed successfully")
	return nil
}

// DestroyAll destroys multiple VM instances
// Attempts to destroy all VMs even if some fail (best-effort cleanup)
// Returns aggregated errors using errors.Join
func (o *VMOrchestrator) DestroyAll(ctx context.Context, instances []*VMInstance) error {
	var errs []error

	for _, instance := range instances {
		if instance == nil {
			continue
		}
		if err := o.DestroyVM(ctx, instance); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// RecordEvent records a VM lifecycle event for timeline tracking
func (o *VMOrchestrator) RecordEvent(vmName, eventType, details string) {
	event := TestEvent{
		Timestamp: time.Now(),
		VMName:    vmName,
		EventType: eventType,
		Details:   details,
	}
	o.events = append(o.events, event)
}

// GetEvents returns all recorded events
func (o *VMOrchestrator) GetEvents() []TestEvent {
	return o.events
}

// specToVMMConfig converts a VMSpec to vmm.VMConfig
func (o *VMOrchestrator) specToVMMConfig(spec VMSpec) vmm.VMConfig {
	cfg := vmm.VMConfig{
		Name:     spec.Name,
		Network:  o.network,
		UserData: cloudinit.UserData{}, // Empty UserData for network boot VMs
	}

	// Parse memory (convert string to MB)
	if spec.Memory != "" {
		// Assume memory is already in MB format (e.g., "2048")
		var memoryMB uint
		_, _ = fmt.Sscanf(spec.Memory, "%d", &memoryMB)
		if memoryMB > 0 {
			cfg.MemoryMB = memoryMB
		}
	}

	// Set VCPUs
	if spec.VCPUs > 0 {
		cfg.VCPUs = uint(spec.VCPUs)
	}

	// Set disk configuration if specified
	if spec.Disk != nil {
		cfg.ImageQCOW2Path = spec.Disk.Image
		if spec.Disk.Size != "" {
			cfg.DiskSize = spec.Disk.Size
		}
	}

	return cfg
}
