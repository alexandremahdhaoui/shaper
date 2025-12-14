//go:build unit

// Copyright 2024 Alexandre Mahdhaoui
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tftp

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockReaderFrom implements io.ReaderFrom for testing
type mockReaderFrom struct {
	data bytes.Buffer
}

func (m *mockReaderFrom) ReadFrom(r io.Reader) (int64, error) {
	return m.data.ReadFrom(r)
}

func TestNew(t *testing.T) {
	tmpDir := t.TempDir()

	config := &ServerConfig{
		Address:  ":0", // Use port 0 for testing
		RootDir:  tmpDir,
		ReadOnly: true,
		Timeout:  5,
		Retries:  5,
	}

	server, err := New(config, slog.Default())
	require.NoError(t, err)
	assert.NotNil(t, server)
	assert.Equal(t, config, server.config)
}

func TestNew_InvalidRootDir(t *testing.T) {
	config := &ServerConfig{
		Address:  ":0",
		RootDir:  "/nonexistent/directory",
		ReadOnly: true,
	}

	server, err := New(config, slog.Default())
	assert.Error(t, err)
	assert.Nil(t, server)
	assert.Contains(t, err.Error(), "root directory")
}

func TestHandleRead_ValidFile(t *testing.T) {
	// Create temporary directory and test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := []byte("Hello, TFTP!")
	err := os.WriteFile(testFile, testContent, 0o644)
	require.NoError(t, err)

	// Create server
	config := &ServerConfig{
		Address:  ":0",
		RootDir:  tmpDir,
		ReadOnly: true,
	}
	server, err := New(config, slog.Default())
	require.NoError(t, err)

	// Test read
	rf := &mockReaderFrom{}
	err = server.handleRead("test.txt", rf)
	assert.NoError(t, err)
	assert.Equal(t, testContent, rf.data.Bytes())
}

func TestHandleRead_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	config := &ServerConfig{
		Address:  ":0",
		RootDir:  tmpDir,
		ReadOnly: true,
	}
	server, err := New(config, slog.Default())
	require.NoError(t, err)

	rf := &mockReaderFrom{}
	err = server.handleRead("nonexistent.txt", rf)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrFileNotFound)
}

func TestHandleRead_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file outside the root directory
	outsideDir := filepath.Join(tmpDir, "outside")
	err := os.Mkdir(outsideDir, 0o755)
	require.NoError(t, err)

	rootDir := filepath.Join(tmpDir, "root")
	err = os.Mkdir(rootDir, 0o755)
	require.NoError(t, err)

	// Try to access file outside root with path traversal
	config := &ServerConfig{
		Address:  ":0",
		RootDir:  rootDir,
		ReadOnly: true,
	}
	server, err := New(config, slog.Default())
	require.NoError(t, err)

	testCases := []struct {
		name     string
		filename string
	}{
		{
			name:     "parent directory traversal",
			filename: "../outside/file.txt",
		},
		{
			name:     "absolute path",
			filename: "/etc/passwd",
		},
		{
			name:     "multiple parent traversals",
			filename: "../../outside/file.txt",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rf := &mockReaderFrom{}
			err := server.handleRead(tc.filename, rf)
			assert.Error(t, err)
			// Should either be path traversal error or file not found
			// (depending on how the path is sanitized)
		})
	}
}

func TestHandleRead_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty.txt")
	err := os.WriteFile(testFile, []byte{}, 0o644)
	require.NoError(t, err)

	config := &ServerConfig{
		Address:  ":0",
		RootDir:  tmpDir,
		ReadOnly: true,
	}
	server, err := New(config, slog.Default())
	require.NoError(t, err)

	rf := &mockReaderFrom{}
	err = server.handleRead("empty.txt", rf)
	assert.NoError(t, err)
	assert.Empty(t, rf.data.Bytes())
}

func TestNewDefaultConfig(t *testing.T) {
	config := NewDefaultConfig()
	assert.NotNil(t, config)
	assert.Equal(t, ":69", config.Address)
	assert.Equal(t, "/var/lib/shaper/tftp", config.RootDir)
	assert.True(t, config.ReadOnly)
	assert.Equal(t, 5, config.Timeout)
	assert.Equal(t, 5, config.Retries)
}
