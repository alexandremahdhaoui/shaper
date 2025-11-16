// Package orchestration provides components for E2E test orchestration.
package orchestration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"text/template"
	"time"
)

var (
	// ErrKubeconfigRequired is returned when kubeconfig is empty
	ErrKubeconfigRequired = errors.New("kubeconfig path is required")

	// ErrResourceTimeout is returned when resource readiness check times out
	ErrResourceTimeout = errors.New("timeout waiting for resource to be ready")

	// ErrInvalidYAML is returned when resource YAML is invalid
	ErrInvalidYAML = errors.New("invalid resource YAML")
)

// ResourceApplier applies Kubernetes resources to a test environment.
type ResourceApplier struct {
	kubeconfig string
	namespace  string
}

// K8sResource represents a Kubernetes resource to apply.
type K8sResource struct {
	Kind      string
	Name      string
	Namespace string
	YAML      []byte
}

// AppliedResource tracks a Kubernetes resource that was created.
type AppliedResource struct {
	Kind      string
	Name      string
	Namespace string
	Status    string // "created", "failed", "skipped"
	CreatedAt time.Time
	Error     string // Error message if failed
}

// TemplateVars contains variables for template substitution.
type TemplateVars struct {
	VMName string
	VMUUID string
	VMMAC  string
	VMIP   string
}

// NewResourceApplier creates a new ResourceApplier.
func NewResourceApplier(kubeconfig, namespace string) *ResourceApplier {
	return &ResourceApplier{
		kubeconfig: kubeconfig,
		namespace:  namespace,
	}
}

// ApplyResource applies a single Kubernetes resource.
// It applies the resource and waits for it to exist (not for readiness conditions).
func (a *ResourceApplier) ApplyResource(ctx context.Context, resource K8sResource) (*AppliedResource, error) {
	if a.kubeconfig == "" {
		return nil, ErrKubeconfigRequired
	}

	result := &AppliedResource{
		Kind:      resource.Kind,
		Name:      resource.Name,
		Namespace: resource.Namespace,
		CreatedAt: time.Now(),
	}

	// Use resource namespace if specified, otherwise use applier's default
	namespace := resource.Namespace
	if namespace == "" {
		namespace = a.namespace
	}

	// Apply resource via kubectl
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", a.kubeconfig,
		"apply", "-n", namespace, "-f", "-")
	cmd.Stdin = bytes.NewReader(resource.YAML)

	output, err := cmd.CombinedOutput()
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("kubectl apply failed: %v, output: %s", err, string(output))
		return result, fmt.Errorf("applying %s/%s: %w: %s", resource.Kind, resource.Name, err, string(output))
	}

	// Wait for resource to exist
	if err := a.waitForResourceExists(ctx, resource.Kind, resource.Name, namespace); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("resource wait failed: %v", err)
		return result, err
	}

	result.Status = "created"
	return result, nil
}

// ApplyResources applies multiple Kubernetes resources in order.
// It returns a slice of AppliedResource tracking the status of each resource.
// If any resource fails, it stops and returns the error along with partial results.
func (a *ResourceApplier) ApplyResources(ctx context.Context, resources []K8sResource) ([]*AppliedResource, error) {
	var results []*AppliedResource
	var errs []error

	for _, resource := range resources {
		result, err := a.ApplyResource(ctx, resource)
		if result != nil {
			results = append(results, result)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to apply %s/%s: %w", resource.Kind, resource.Name, err))
			// Continue to track remaining resources as skipped
			for i := len(results); i < len(resources); i++ {
				results = append(results, &AppliedResource{
					Kind:      resources[i].Kind,
					Name:      resources[i].Name,
					Namespace: resources[i].Namespace,
					Status:    "skipped",
					CreatedAt: time.Now(),
					Error:     "previous resource failed",
				})
			}
			break
		}
	}

	if len(errs) > 0 {
		return results, errors.Join(errs...)
	}

	return results, nil
}

// DeleteResource deletes a single Kubernetes resource.
func (a *ResourceApplier) DeleteResource(ctx context.Context, resource K8sResource) error {
	if a.kubeconfig == "" {
		return ErrKubeconfigRequired
	}

	namespace := resource.Namespace
	if namespace == "" {
		namespace = a.namespace
	}

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", a.kubeconfig,
		"delete", resource.Kind, resource.Name, "-n", namespace,
		"--ignore-not-found=true")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("deleting %s/%s: %w: %s", resource.Kind, resource.Name, err, string(output))
	}

	return nil
}

// DeleteAll deletes all provided resources.
// It attempts to delete all resources even if some fail.
// Returns aggregated errors if any deletions fail.
func (a *ResourceApplier) DeleteAll(ctx context.Context, resources []K8sResource) error {
	var errs []error

	for _, resource := range resources {
		if err := a.DeleteResource(ctx, resource); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// GetResource retrieves a Kubernetes resource by kind and name.
func (a *ResourceApplier) GetResource(ctx context.Context, kind, name, namespace string) ([]byte, error) {
	if a.kubeconfig == "" {
		return nil, ErrKubeconfigRequired
	}

	if namespace == "" {
		namespace = a.namespace
	}

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", a.kubeconfig,
		"get", kind, name, "-n", namespace, "-o", "yaml")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("getting %s/%s: %w: %s", kind, name, err, string(output))
	}

	return output, nil
}

// RenderTemplate renders a YAML template with provided variables.
// Supports Go template syntax with variables: {{.VMName}}, {{.VMUUID}}, {{.VMMAC}}, {{.VMIP}}
func (a *ResourceApplier) RenderTemplate(yamlTemplate string, vars TemplateVars) ([]byte, error) {
	tmpl, err := template.New("resource").Parse(yamlTemplate)
	if err != nil {
		return nil, fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return nil, fmt.Errorf("executing template: %w", err)
	}

	return buf.Bytes(), nil
}

// waitForResourceExists waits for a resource to exist in Kubernetes.
// It polls kubectl get until the resource exists or timeout is reached.
// Timeout: 60 seconds, Poll interval: 2 seconds (from NEW-TASKS.md)
func (a *ResourceApplier) waitForResourceExists(ctx context.Context, kind, name, namespace string) error {
	const (
		timeout      = 60 * time.Second
		pollInterval = 2 * time.Second
	)

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	// Immediate first check
	cmd := exec.CommandContext(waitCtx, "kubectl", "--kubeconfig", a.kubeconfig,
		"get", kind, name, "-n", namespace, "-o", "name")
	if err := cmd.Run(); err == nil {
		return nil // Resource exists
	}

	// Poll until exists or timeout
	for {
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("%w: %s/%s in namespace %s", ErrResourceTimeout, kind, name, namespace)
		case <-ticker.C:
			cmd := exec.CommandContext(waitCtx, "kubectl", "--kubeconfig", a.kubeconfig,
				"get", kind, name, "-n", namespace, "-o", "name")
			if err := cmd.Run(); err == nil {
				return nil // Resource exists
			}
			// Resource doesn't exist yet, continue waiting
		}
	}
}

// ValidateYAML performs basic YAML validation.
func (a *ResourceApplier) ValidateYAML(yamlData []byte) error {
	// Check if YAML is not empty
	if len(yamlData) == 0 {
		return fmt.Errorf("%w: empty YAML", ErrInvalidYAML)
	}

	// Check if YAML contains at least "kind:" and "metadata:" fields
	yamlStr := string(yamlData)
	if !strings.Contains(yamlStr, "kind:") {
		return fmt.Errorf("%w: missing 'kind' field", ErrInvalidYAML)
	}
	if !strings.Contains(yamlStr, "metadata:") {
		return fmt.Errorf("%w: missing 'metadata' field", ErrInvalidYAML)
	}

	return nil
}
