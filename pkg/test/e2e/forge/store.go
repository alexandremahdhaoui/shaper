//go:build e2e

package forge

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e/infrastructure"
)

var (
	// ErrEnvironmentNotFound indicates the requested environment ID was not found
	ErrEnvironmentNotFound = errors.New("environment not found")
	// ErrStoreCorrupted indicates the store file is corrupted or invalid
	ErrStoreCorrupted = errors.New("store corrupted")
)

// EnvironmentStore manages persistence of infrastructure state
type EnvironmentStore interface {
	Save(env *infrastructure.InfrastructureState) error
	Load(id string) (*infrastructure.InfrastructureState, error)
	List() ([]*infrastructure.InfrastructureState, error)
	Delete(id string) error
}

// JSONEnvironmentStore implements EnvironmentStore using JSON files
// Each environment is stored as a separate JSON file
type JSONEnvironmentStore struct {
	storeDir string
	mu       sync.RWMutex // Protect concurrent access
}

// NewJSONEnvironmentStore creates a new JSON-based environment store
func NewJSONEnvironmentStore(storeDir string) (*JSONEnvironmentStore, error) {
	// Create store directory if it doesn't exist
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create store directory: %w", err)
	}

	return &JSONEnvironmentStore{
		storeDir: storeDir,
	}, nil
}

// Save persists an environment state to disk
func (s *JSONEnvironmentStore) Save(env *infrastructure.InfrastructureState) error {
	if env == nil {
		return errors.New("environment state is nil")
	}
	if env.ID == "" {
		return errors.New("environment ID is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Marshal to JSON
	data, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal environment: %w", err)
	}

	// Write to file
	filePath := s.getFilePath(env.ID)
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write environment file: %w", err)
	}

	return nil
}

// Load retrieves an environment state from disk
func (s *JSONEnvironmentStore) Load(id string) (*infrastructure.InfrastructureState, error) {
	if id == "" {
		return nil, errors.New("environment ID is empty")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	filePath := s.getFilePath(id)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, ErrEnvironmentNotFound
	}

	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read environment file: %w", err)
	}

	// Unmarshal JSON
	var env infrastructure.InfrastructureState
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, errors.Join(err, ErrStoreCorrupted)
	}

	return &env, nil
}

// List returns all stored environments
func (s *JSONEnvironmentStore) List() ([]*infrastructure.InfrastructureState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Read directory entries
	entries, err := os.ReadDir(s.storeDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read store directory: %w", err)
	}

	var environments []*infrastructure.InfrastructureState

	// Load each environment file
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only process .json files
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		// Extract ID from filename (remove .json extension)
		id := entry.Name()[:len(entry.Name())-5]

		// Read file
		filePath := s.getFilePath(id)
		data, err := os.ReadFile(filePath)
		if err != nil {
			// Skip files that can't be read
			continue
		}

		// Unmarshal JSON
		var env infrastructure.InfrastructureState
		if err := json.Unmarshal(data, &env); err != nil {
			// Skip corrupted files
			continue
		}

		environments = append(environments, &env)
	}

	return environments, nil
}

// Delete removes an environment from the store
func (s *JSONEnvironmentStore) Delete(id string) error {
	if id == "" {
		return errors.New("environment ID is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	filePath := s.getFilePath(id)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return ErrEnvironmentNotFound
	}

	// Remove file
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("failed to delete environment file: %w", err)
	}

	return nil
}

// getFilePath returns the file path for an environment ID
func (s *JSONEnvironmentStore) getFilePath(id string) string {
	return filepath.Join(s.storeDir, id+".json")
}
