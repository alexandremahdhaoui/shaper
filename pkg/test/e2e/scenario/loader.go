package scenario

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Loader loads test scenarios from YAML files.
type Loader struct {
	// basePath is the base directory for resolving relative paths
	basePath string
}

// NewLoader creates a new scenario loader.
// basePath is used to resolve relative scenario file paths.
// If basePath is empty, the current working directory is used.
func NewLoader(basePath string) *Loader {
	if basePath == "" {
		basePath = "."
	}
	return &Loader{
		basePath: basePath,
	}
}

// Load loads a test scenario from a YAML file.
// The path can be absolute or relative to the loader's basePath.
// Returns the parsed and validated TestScenario or an error.
func (l *Loader) Load(path string) (*TestScenario, error) {
	// Resolve the file path
	resolvedPath, err := l.resolvePath(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve scenario path: %w", err)
	}

	// Read the file
	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read scenario file %s: %w", resolvedPath, err)
	}

	// Parse YAML
	var scenario TestScenario
	if err := yaml.Unmarshal(data, &scenario); err != nil {
		return nil, fmt.Errorf("failed to parse YAML from %s: %w", resolvedPath, err)
	}

	// Validate the scenario
	if err := Validate(&scenario); err != nil {
		return nil, fmt.Errorf("scenario validation failed for %s: %w", resolvedPath, err)
	}

	return &scenario, nil
}

// LoadMultiple loads multiple test scenarios from YAML files.
// Returns all successfully loaded scenarios and any errors encountered.
func (l *Loader) LoadMultiple(paths []string) ([]*TestScenario, []error) {
	scenarios := make([]*TestScenario, 0, len(paths))
	var errs []error

	for _, path := range paths {
		scenario, err := l.Load(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to load %s: %w", path, err))
			continue
		}
		scenarios = append(scenarios, scenario)
	}

	return scenarios, errs
}

// resolvePath resolves a file path relative to the loader's basePath.
// If the path is absolute, it is returned as-is.
// If the path is relative, it is joined with basePath.
func (l *Loader) resolvePath(path string) (string, error) {
	// If path is already absolute, return it
	if filepath.IsAbs(path) {
		return path, nil
	}

	// Join with basePath
	resolvedPath := filepath.Join(l.basePath, path)

	// Verify the file exists
	if _, err := os.Stat(resolvedPath); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("scenario file does not exist: %s", resolvedPath)
		}
		return "", fmt.Errorf("failed to stat scenario file %s: %w", resolvedPath, err)
	}

	return resolvedPath, nil
}

// DefaultScenarioPath returns the default path for scenario files.
// This is typically "test/e2e/scenarios" relative to the project root.
func DefaultScenarioPath() string {
	return "test/e2e/scenarios"
}
