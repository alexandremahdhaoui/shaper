/*
Copyright 2024 Alexandre Mahdhaoui

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tftp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pin/tftp/v3"
)

var (
	// ErrPathTraversal is returned when a file path attempts to escape the root directory
	ErrPathTraversal = errors.New("path traversal attempt detected")

	// ErrFileNotFound is returned when a requested file doesn't exist
	ErrFileNotFound = errors.New("file not found")

	// ErrWriteNotAllowed is returned when a write operation is attempted on a read-only server
	ErrWriteNotAllowed = errors.New("write operations not allowed")
)

// Server is a TFTP server that serves iPXE boot files
type Server struct {
	config *ServerConfig
	server *tftp.Server
	logger *slog.Logger
}

// New creates a new TFTP server with the given configuration
func New(config *ServerConfig, logger *slog.Logger) (*Server, error) {
	if config == nil {
		config = NewDefaultConfig()
	}

	if logger == nil {
		logger = slog.Default()
	}

	// Validate root directory exists
	if _, err := os.Stat(config.RootDir); err != nil {
		return nil, fmt.Errorf("root directory %s: %w", config.RootDir, err)
	}

	s := &Server{
		config: config,
		logger: logger,
	}

	// Create TFTP server with read handler
	s.server = tftp.NewServer(s.handleRead, nil)
	s.server.SetTimeout(time.Duration(config.Timeout) * time.Second)

	return s, nil
}

// Start starts the TFTP server
func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("Starting TFTP server",
		"address", s.config.Address,
		"rootDir", s.config.RootDir,
		"readOnly", s.config.ReadOnly)

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		err := s.server.ListenAndServe(s.config.Address)
		if err != nil {
			errCh <- fmt.Errorf("TFTP server error: %w", err)
		}
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		s.logger.Info("Shutting down TFTP server")
		s.server.Shutdown()
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

// Stop stops the TFTP server
func (s *Server) Stop() error {
	s.logger.Info("Stopping TFTP server")
	s.server.Shutdown()
	return nil
}

// handleRead handles TFTP read requests
func (s *Server) handleRead(filename string, rf io.ReaderFrom) error {
	// Sanitize the filename to prevent path traversal
	cleanFilename := filepath.Clean(filename)

	// Remove leading slashes
	cleanFilename = strings.TrimLeft(cleanFilename, "/")

	// Build full path
	fullPath := filepath.Join(s.config.RootDir, cleanFilename)

	// Verify the path is still within root directory (prevent path traversal)
	if !strings.HasPrefix(fullPath, s.config.RootDir) {
		s.logger.Warn("Path traversal attempt detected",
			"filename", filename,
			"cleanFilename", cleanFilename,
			"fullPath", fullPath)
		return ErrPathTraversal
	}

	// Open the file
	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.logger.Warn("File not found",
				"filename", filename,
				"fullPath", fullPath)
			return ErrFileNotFound
		}
		s.logger.Error("Failed to open file",
			"filename", filename,
			"error", err)
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = file.Close() }()

	// Get file info for logging
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	s.logger.Info("Serving file",
		"filename", filename,
		"size", fileInfo.Size())

	// Send the file to the client
	n, err := rf.ReadFrom(file)
	if err != nil {
		s.logger.Error("Failed to send file",
			"filename", filename,
			"error", err)
		return fmt.Errorf("failed to send file: %w", err)
	}

	s.logger.Info("File sent successfully",
		"filename", filename,
		"bytes", n)

	return nil
}
