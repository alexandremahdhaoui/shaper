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

package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/alexandremahdhaoui/shaper/internal/adapter"
	driverwebhook "github.com/alexandremahdhaoui/shaper/internal/driver/webhook"
	"github.com/alexandremahdhaoui/shaper/internal/util/gracefulshutdown"
	"github.com/alexandremahdhaoui/shaper/internal/util/httputil"
)

const (
	Name = "shaper-webhook"
)

var (
	Version        = "dev" //nolint:gochecknoglobals // set by ldflags
	CommitSHA      = "n/a" //nolint:gochecknoglobals // set by ldflags
	BuildTimestamp = "n/a" //nolint:gochecknoglobals // set by ldflags
)

// ------------------------------------------------- Main ----------------------------------------------------------- //

func main() {
	_, _ = fmt.Fprintf(
		os.Stdout,
		"Starting %s version %s (%s) %s\n",
		Name,
		Version,
		CommitSHA,
		BuildTimestamp,
	)

	// --------------------------------------------- Graceful Shutdown ---------------------------------------------- //

	gs := gracefulshutdown.New(Name)
	ctx := gs.Context()

	// --------------------------------------------- Config --------------------------------------------------------- //

	config, err := loadConfig(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "loading configuration", "error", err.Error())
		gs.Shutdown(1)
	}

	// --------------------------------------------- Client --------------------------------------------------------- //

	restConfig, err := newKubeRestConfig(config.KubeconfigPath)
	if err != nil {
		slog.ErrorContext(ctx, "creating kube rest config", "error", err.Error())
		gs.Shutdown(1)
	}

	cl, err := newKubeClient(restConfig)
	if err != nil {
		slog.ErrorContext(ctx, "creating kube client", "error", err.Error())
		gs.Shutdown(1)
	}

	// --------------------------------------------- Adapter -------------------------------------------------------- //

	assignment := adapter.NewAssignment(cl, config.AssignmentNamespace)
	profile := adapter.NewProfile(cl, config.ProfileNamespace)

	// --------------------------------------------- Manager -------------------------------------------------------- //

	// Create controller-runtime manager with webhook server options
	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{ //nolint:exhaustruct
		Scheme: cl.Scheme(),
		WebhookServer: webhook.NewServer(webhook.Options{ //nolint:exhaustruct
			Port:     config.WebhookServer.Port,
			CertDir:  config.WebhookServer.CertDir,
			CertName: config.WebhookServer.CertName,
			KeyName:  config.WebhookServer.KeyName,
		}),
	})
	if err != nil {
		slog.ErrorContext(ctx, "creating controller-runtime manager", "error", err.Error())
		gs.Shutdown(1)
	}

	// --------------------------------------------- Webhooks ------------------------------------------------------- //

	// Create webhook instances
	assignmentWebhook := driverwebhook.NewAssignment(assignment, profile)
	profileWebhook := driverwebhook.NewProfile()

	// Set up webhook server
	if err := setupWebhookServer(mgr, assignmentWebhook, profileWebhook); err != nil {
		slog.ErrorContext(ctx, "setting up webhook server", "error", err.Error())
		gs.Shutdown(1)
	}

	// --------------------------------------------- Metrics & Probes ----------------------------------------------- //

	metricsServer := setupMetricsServer(config)
	probesServer := setupProbesServer(config)

	// Start metrics and probes servers
	go httputil.Serve(map[string]*http.Server{
		"metrics": metricsServer,
		"probes":  probesServer,
	}, gs)

	// --------------------------------------------- Run Manager ---------------------------------------------------- //

	// Start manager in goroutine
	gs.WaitGroup().Add(1)
	go func() {
		defer gs.WaitGroup().Done()

		slog.InfoContext(ctx, "starting webhook server", "port", config.WebhookServer.Port)
		if err := mgr.Start(ctx); err != nil {
			slog.ErrorContext(ctx, "running manager", "error", err.Error())
			gs.Shutdown(1)
		}
	}()

	// Wait for context to be done
	<-ctx.Done()

	slog.Info("✅ gracefully stopped", "binary", Name)
}
