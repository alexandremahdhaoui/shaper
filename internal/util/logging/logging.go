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

// Package logging provides shared logging utilities for all shaper binaries.
// It uses log/slog as the standard library logger and bridges it to logr
// for controller-runtime compatibility.
package logging

import (
	"log/slog"
	"os"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// Options configures the logger behavior.
type Options struct {
	// Development enables development mode logging (more verbose, human-readable).
	Development bool

	// Level sets the minimum log level. Defaults to slog.LevelInfo.
	Level slog.Level
}

// DefaultOptions returns the default logging options.
func DefaultOptions() Options {
	return Options{
		Development: false,
		Level:       slog.LevelInfo,
	}
}

// Setup configures both the standard library slog logger and controller-runtime logger.
// This must be called early in main() before using any logging or controller-runtime features.
//
// It:
//  1. Sets up a slog JSON handler as the default logger
//  2. Configures controller-runtime to use zap logger (which integrates with slog)
//
// The controller-runtime logger is set to prevent panics when controller-runtime
// components try to log before SetLogger is called.
func Setup(opts Options) logr.Logger {
	// Configure slog with JSON handler for structured logging
	var handler slog.Handler
	if opts.Development {
		// Use text handler for development (more readable)
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: opts.Level,
		})
	} else {
		// Use JSON handler for production (structured, machine-readable)
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: opts.Level,
		})
	}
	slog.SetDefault(slog.New(handler))

	// Configure controller-runtime logger using zap
	// zap is the standard logger for controller-runtime and integrates well
	zapOpts := zap.Options{
		Development: opts.Development,
	}
	logger := zap.New(zap.UseFlagOptions(&zapOpts))
	ctrl.SetLogger(logger)

	return logger
}

// SetupDefault sets up logging with default options.
// Convenience function for simple cases.
func SetupDefault() logr.Logger {
	return Setup(DefaultOptions())
}

// SetupDevelopment sets up logging in development mode.
// Uses text handler and more verbose output.
func SetupDevelopment() logr.Logger {
	return Setup(Options{
		Development: true,
		Level:       slog.LevelDebug,
	})
}
