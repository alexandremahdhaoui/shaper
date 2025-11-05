//go:build unit

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

package gracefulshutdown_test

import (
	"sync"
	"testing"
	"time"

	"github.com/alexandremahdhaoui/shaper/internal/util/gracefulshutdown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNew verifies that New() creates a properly initialized GracefulShutdown struct.
func TestNew(t *testing.T) {
	// Create GracefulShutdown with mock exit
	mockExit := func(code int) {}
	gs := gracefulshutdown.NewWithExit("test-server", mockExit)
	require.NotNil(t, gs, "GracefulShutdown should not be nil")

	// Verify Context is not nil
	ctx := gs.Context()
	assert.NotNil(t, ctx, "Context should not be nil")

	// Verify context is not already cancelled
	select {
	case <-ctx.Done():
		t.Fatal("Context should not be cancelled initially")
	default:
		// Expected: context not done
	}

	// Verify CancelFunc is not nil
	cancel := gs.CancelFunc()
	assert.NotNil(t, cancel, "CancelFunc should not be nil")

	// Verify WaitGroup is not nil
	wg := gs.WaitGroup()
	assert.NotNil(t, wg, "WaitGroup should not be nil")
}

// TestGracefulShutdown_Context verifies that the context returned by Context() can be cancelled.
func TestGracefulShutdown_Context(t *testing.T) {
	// Create GracefulShutdown with mock exit
	mockExit := func(code int) {}
	gs := gracefulshutdown.NewWithExit("test-server", mockExit)

	// Get context
	ctx := gs.Context()
	require.NotNil(t, ctx)

	// Verify context is not cancelled initially
	assert.NoError(t, ctx.Err(), "context should not be cancelled initially")

	// Get cancel function and call it
	cancel := gs.CancelFunc()
	require.NotNil(t, cancel)
	cancel()

	// Verify context is now cancelled
	<-ctx.Done()
	assert.Error(t, ctx.Err(), "context should be cancelled after calling cancel")
}

// TestGracefulShutdown_WaitGroup verifies WaitGroup() returns a functional wait group.
func TestGracefulShutdown_WaitGroup(t *testing.T) {
	// Create GracefulShutdown with mock exit
	mockExit := func(code int) {}
	gs := gracefulshutdown.NewWithExit("test-server", mockExit)

	// Get wait group
	wg := gs.WaitGroup()
	require.NotNil(t, wg)

	// Test wait group functionality
	completed := false
	wg.Add(1)
	go func() {
		defer wg.Done()
		completed = true
	}()

	// Wait for goroutine to complete
	wg.Wait()

	// Verify goroutine completed
	assert.True(t, completed, "goroutine should have completed")
}

// TestGracefulShutdown_Shutdown verifies Shutdown() method calls exit function with correct code.
func TestGracefulShutdown_Shutdown(t *testing.T) {
	tests := []struct {
		name       string
		exitCode   int
		wgAddCount int // Number of goroutines to simulate
	}{
		{
			name:     "shutdown with exit code 0",
			exitCode: 0,
		},
		{
			name:     "shutdown with exit code 1",
			exitCode: 1,
		},
		{
			name:       "shutdown waits for waitgroup",
			exitCode:   0,
			wgAddCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture exit code instead of exiting
			var capturedExitCode int
			exitCalled := false
			mockExit := func(code int) {
				capturedExitCode = code
				exitCalled = true
			}

			gs := gracefulshutdown.NewWithExit("test", mockExit)

			// Simulate goroutines if needed
			if tt.wgAddCount > 0 {
				for i := 0; i < tt.wgAddCount; i++ {
					gs.WaitGroup().Add(1)
					go func() {
						time.Sleep(10 * time.Millisecond)
						gs.WaitGroup().Done()
					}()
				}
			}

			// Call Shutdown
			gs.Shutdown(tt.exitCode)

			// Verify exit was called with correct code
			assert.True(t, exitCalled, "exit function should be called")
			assert.Equal(t, tt.exitCode, capturedExitCode)

			// Verify context was cancelled
			assert.Error(t, gs.Context().Err(), "context should be cancelled")
		})
	}
}

// TestGracefulShutdown_ShutdownIdempotency verifies Shutdown() is only executed once.
func TestGracefulShutdown_ShutdownIdempotency(t *testing.T) {
	exitCallCount := 0
	var mu sync.Mutex
	mockExit := func(code int) {
		mu.Lock()
		defer mu.Unlock()
		exitCallCount++
	}

	gs := gracefulshutdown.NewWithExit("test", mockExit)

	// Call Shutdown multiple times concurrently
	const concurrentCalls = 10
	var wg sync.WaitGroup
	for i := 0; i < concurrentCalls; i++ {
		wg.Add(1)
		go func(exitCode int) {
			defer wg.Done()
			gs.Shutdown(exitCode)
		}(i)
	}

	wg.Wait()

	// Verify exit was called exactly once
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, exitCallCount,
		"Shutdown should be idempotent - exit should be called exactly once")
}
