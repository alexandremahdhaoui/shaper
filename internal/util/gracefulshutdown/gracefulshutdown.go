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

package gracefulshutdown

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// GracefulShutdown is a struct that holds the context, cancel function, name, mutex, and wait group for a graceful
// shutdown.
type GracefulShutdown struct {
	ctx    context.Context
	cancel context.CancelFunc
	name   string

	once sync.Once
	wg   *sync.WaitGroup

	// exitFunc allows injecting exit behavior for testing
	exitFunc func(int)
}

// NewWithExit creates a new GracefulShutdown struct with a custom exit function.
// This is primarily useful for testing where os.Exit() would terminate the test process.
func NewWithExit(name string, exitFunc func(int)) *GracefulShutdown {
	// 1. initialize a new cancelable context.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt, os.Kill)

	// 2. initialize a new wait group.
	wg := &sync.WaitGroup{}

	// 3. create the GracefulShutdown struct.
	gs := &GracefulShutdown{
		ctx:      ctx,
		cancel:   cancel,
		name:     name,
		wg:       wg,
		exitFunc: exitFunc,
	}

	// 4. Ensure gs.Shutdown is always called at least once when the context is done.
	go func() {
		<-ctx.Done()
		gs.Shutdown(0)
	}()

	return gs
}

// New creates a new GracefulShutdown struct initializing a sync.WaitGroup and a new context.Context cancelable by a
// CancelFunc, a SIGTERM, SIGINT or SIGKILL.
func New(name string) *GracefulShutdown {
	return NewWithExit(name, os.Exit)
}

// Shutdown shuts down the application gracefully.
func (s *GracefulShutdown) Shutdown(exitCode int) {
	// Use sync.Once to ensure shutdown logic only executes once, even if called multiple times.
	s.once.Do(func() {
		// 1. Print a log line.
		slog.InfoContext(s.ctx, fmt.Sprintf("⌛ gracefully shutting down %s", s.name))

		// 2. Cancel the context.
		s.cancel()

		// 3. Wait until all goroutines which incremented the wait group are done.
		s.wg.Wait()

		// 4. Exit using the injected function.
		s.exitFunc(exitCode)
	})
}

// Context returns the context of the graceful shutdown.
func (s *GracefulShutdown) Context() context.Context {
	return s.ctx
}

// CancelFunc returns the cancel function of the graceful shutdown.
func (s *GracefulShutdown) CancelFunc() context.CancelFunc {
	return s.cancel
}

// WaitGroup returns the wait group of the graceful shutdown.
func (s *GracefulShutdown) WaitGroup() *sync.WaitGroup {
	return s.wg
}
