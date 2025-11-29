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

package httputil

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/alexandremahdhaoui/shaper/internal/util/gracefulshutdown"
	"github.com/alexandremahdhaoui/shaper/pkg/constants"
)

// Serve serves the given servers and handles graceful shutdown.
func Serve(servers map[string]*http.Server, gs *gracefulshutdown.GracefulShutdown) {
	// 1. Run the servers.
	for name, server := range servers {
		ctx := context.WithValue(gs.Context(), constants.ServerNameContextKey, name)

		// sets the base context to be the GracefulShutdown's context.
		server.BaseContext = func(_ net.Listener) context.Context {
			return ctx
		}

		gs.WaitGroup().Add(1)

		go func() {
			if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.ErrorContext(ctx, "❌ received error", "error", err)

				// we need to call Done() before requesting the shutdown. Otherwise, the WaitGroup will never decrement.
				gs.WaitGroup().Done()
				gs.Shutdown(1) // Initiate a graceful shutdown. This call is blocking and awaits for wg.

				return
			}

			gs.WaitGroup().Done()

			// The server stopped running without errors, thus we initiate a graceful shutdown if none was previously
			// initiated.
			gs.Shutdown(0)
		}()
	}

	// 2. Signal that all Add() calls have been made.
	// This allows the auto-shutdown goroutine to proceed when context is cancelled.
	gs.Ready()

	// 3. Await context is done.
	<-gs.Context().Done()

	// 4. Gracefully shutdown each server.
	for name, server := range servers {
		go func() {
			ctx := context.WithValue(context.Background(), constants.ServerNameContextKey, name)

			ctx, cancel := context.WithDeadline(ctx, time.Now().Add(1*time.Minute)) // 1 min deadline.
			defer cancel()

			if err := server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.ErrorContext(ctx, "❌ received error while shutting down server", "error", err)

				return
			}

			slog.Info("✅ gracefully shut down server")
		}()
	}
}
