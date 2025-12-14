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

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/alexandremahdhaoui/shaper/internal/driver/tftp"
)

const (
	Name = "shaper-tftp"
)

func main() {
	// Setup logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: getLogLevel(),
	}))
	slog.SetDefault(logger)

	logger.Info("Starting shaper-tftp server")

	// Load configuration from environment
	config := &tftp.ServerConfig{
		Address:  getEnv("SHAPER_TFTP_ADDRESS", ":69"),
		RootDir:  getEnv("SHAPER_TFTP_ROOT_DIR", "/var/lib/shaper/tftp"),
		ReadOnly: getEnvBool("SHAPER_TFTP_READ_ONLY", true),
		Timeout:  getEnvInt("SHAPER_TFTP_TIMEOUT", 5),
		Retries:  getEnvInt("SHAPER_TFTP_RETRIES", 5),
	}

	logger.Info("TFTP server configuration",
		"address", config.Address,
		"rootDir", config.RootDir,
		"readOnly", config.ReadOnly)

	// Create TFTP server
	server, err := tftp.New(config, logger)
	if err != nil {
		logger.Error("Failed to create TFTP server", "error", err)
		os.Exit(1)
	}

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		if err := server.Start(ctx); err != nil && err != context.Canceled {
			errCh <- err
		}
	}()

	// Wait for shutdown signal or error
	select {
	case sig := <-sigCh:
		logger.Info("Received shutdown signal", "signal", sig)
		cancel()
		if err := server.Stop(); err != nil {
			logger.Error("Error stopping server", "error", err)
			os.Exit(1)
		}
		logger.Info("Server stopped gracefully")
	case err := <-errCh:
		logger.Error("Server error", "error", err)
		os.Exit(1)
	}
}

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvBool retrieves a boolean environment variable or returns a default value
func getEnvBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value == "true" || value == "1" || value == "yes"
}

// getEnvInt retrieves an integer environment variable or returns a default value
func getEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	var result int
	if _, err := fmt.Sscanf(value, "%d", &result); err != nil {
		return defaultValue
	}
	return result
}

// getLogLevel returns the log level based on environment variable
func getLogLevel() slog.Level {
	level := getEnv("LOG_LEVEL", "info")
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
