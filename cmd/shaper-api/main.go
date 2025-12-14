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
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/alexandremahdhaoui/shaper/internal/util/httputil"
	"github.com/alexandremahdhaoui/shaper/pkg/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/alexandremahdhaoui/shaper/pkg/generated/shaperserver"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/alexandremahdhaoui/shaper/internal/adapter"
	"github.com/alexandremahdhaoui/shaper/internal/controller"
	"github.com/alexandremahdhaoui/shaper/internal/driver/server"
	"github.com/alexandremahdhaoui/shaper/internal/k8s"
	"github.com/alexandremahdhaoui/shaper/internal/types"
	"github.com/alexandremahdhaoui/shaper/internal/util/gracefulshutdown"
	"github.com/alexandremahdhaoui/shaper/internal/util/logging"
)

const (
	Name             = "shaper-api"
	ConfigPathEnvKey = "IPXER_CONFIG_PATH"
)

var (
	scheme         = runtime.NewScheme() //nolint:gochecknoglobals // required for controller-runtime
	Version        = "dev"               //nolint:gochecknoglobals // set by ldflags
	CommitSHA      = "n/a"               //nolint:gochecknoglobals // set by ldflags
	BuildTimestamp = "n/a"               //nolint:gochecknoglobals // set by ldflags
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
}

// Config is used to configure the application.
//
// Some part of the configuration may be passed through environment variables.
type Config struct {
	// Adapters

	// AssignmentNamespace is the namespace where the Assignment resources are located.
	AssignmentNamespace string `json:"assignmentNamespace"`
	// ProfileNamespace is the namespace where the Profile resources are located.
	ProfileNamespace string `json:"profileNamespace"`

	// Kubeconfig

	// KubeconfigPath is the path to the kubeconfig file.
	//
	// It can be set to "in-cluster" to use the in-cluster config.
	KubeconfigPath string `json:"kubeconfigPath"`

	// Controller-runtime Manager options

	// LeaderElection enables or disables leader election for the controller-runtime manager.
	// Defaults to false. Not currently needed for shaper-api (read-only), but available for future HA.
	LeaderElection bool `json:"leaderElection,omitempty"`

	// LeaderElectionID is the name used for leader election.
	// Only used if LeaderElection is true.
	LeaderElectionID string `json:"leaderElectionID,omitempty"`

	// ProbesServer is the configuration for the probes server.
	ProbesServer struct {
		// LivenessPath is the path for the liveness probe.
		LivenessPath string `json:"livenessPath"`
		// ReadinessPath is the path for the readiness probe.
		ReadinessPath string `json:"readinessPath"`
		// Port is the port for the probes server.
		Port int `json:"port"`
	} `json:"probesServer"`

	// MetricsServer is the configuration for the metrics server.
	MetricsServer struct {
		// Path is the path for the metrics server.
		Path string `json:"path"`
		// Port is the port for the metrics server.
		Port int `json:"port"`
	} `json:"metricsServer"`

	// APIServer is the configuration for the API server.
	APIServer struct {
		// Port is the port for the API server.
		Port int `json:"port"`
	} `json:"apiServer"`
}

// ------------------------------------------------- Main ----------------------------------------------------------- //

func main() {
	// Setup logger FIRST before any other operations to ensure controller-runtime
	// doesn't panic when trying to log.
	logging.SetupDefault()

	_, _ = fmt.Fprintf(
		os.Stdout,
		"Starting %s version %s (%s) %s\n",
		Name,
		Version,
		CommitSHA,
		BuildTimestamp,
	)

	gs := gracefulshutdown.New(Name)
	ctx := gs.Context()

	// --------------------------------------------- Config --------------------------------------------------------- //

	shaperConfigPath := os.Getenv(ConfigPathEnvKey)
	if shaperConfigPath == "" {
		slog.ErrorContext(ctx, fmt.Sprintf("environment variable %q must be set", ConfigPathEnvKey))
		gs.Shutdown(1)
	}

	b, err := os.ReadFile(shaperConfigPath)
	if err != nil {
		slog.ErrorContext(ctx, "reading shaper-api configuration file", "error", err.Error())
		gs.Shutdown(1)
	}

	config := new(Config)
	if err = json.Unmarshal(b, config); err != nil {
		slog.ErrorContext(ctx, "parsing shaper-api configuration", "error", err.Error())
		gs.Shutdown(1)
	}

	// --------------------------------------------- Client --------------------------------------------------------- //

	restConfig, err := k8s.NewKubeRestConfig(config.KubeconfigPath)
	if err != nil {
		slog.ErrorContext(ctx, "creating kube rest config", "error", err.Error())
		gs.Shutdown(1)
	}

	// Create controller-runtime Manager for automatic caching
	// Metrics and health probes are disabled because we use custom servers
	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // Disable built-in metrics server (we have custom)
		},
		HealthProbeBindAddress: "", // Disable built-in health probes (we have custom)
		LeaderElection:         config.LeaderElection,
		LeaderElectionID:       config.LeaderElectionID,
		// Cache configuration will be added in Task 6
	})
	if err != nil {
		slog.ErrorContext(ctx, "creating controller-runtime manager", "error", err.Error())
		gs.Shutdown(1)
	}

	// Get cached client from manager for adapters
	cl := mgr.GetClient()

	// Dynamic client is still needed for ObjectRefResolver (dynamic resource access)
	dynCl, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		slog.ErrorContext(ctx, "creating dynamic client", "error", err.Error())
		gs.Shutdown(1)
	}

	// --------------------------------------------- Adapter -------------------------------------------------------- //

	assignment := adapter.NewAssignment(cl, config.AssignmentNamespace)
	profile := adapter.NewProfile(cl, config.ProfileNamespace)

	inlineResolver := adapter.NewInlineResolver()
	objectRefResolver := adapter.NewObjectRefResolver(dynCl)
	webhookResolver := adapter.NewWebhookResolver(objectRefResolver)

	butaneTransformer := adapter.NewButaneTransformer()
	webhookTransformer := adapter.NewWebhookTransformer(objectRefResolver)

	// --------------------------------------------- Controller ----------------------------------------------------- //
	var baseURL string

	mux := controller.NewResolveTransformerMux(
		baseURL,
		map[types.ResolverKind]adapter.Resolver{
			types.InlineResolverKind:    inlineResolver,
			types.ObjectRefResolverKind: objectRefResolver,
			types.WebhookResolverKind:   webhookResolver,
		},
		map[types.TransformerKind]adapter.Transformer{
			types.ButaneTransformerKind:  butaneTransformer,
			types.WebhookTransformerKind: webhookTransformer,
		},
	)

	ipxe := controller.NewIPXE(assignment, profile, mux)
	content := controller.NewContent(profile, mux)

	// --------------------------------------------- App ------------------------------------------------------------ //

	shaperHandler := shaperserver.Handler(shaperserver.NewStrictHandler(
		server.New(ipxe, content),
		nil, // TODO: prometheus middleware
	))

	shaperServer := &http.Server{ //nolint:exhaustruct
		Addr:              fmt.Sprintf(":%d", config.APIServer.Port),
		Handler:           shaperHandler,
		ReadHeaderTimeout: time.Second,
		// TODO: set fields etc...
	}

	// --------------------------------------------- Metrics -------------------------------------------------------- //

	metricsHandler := http.NewServeMux()
	metricsHandler.Handle(config.MetricsServer.Path, promhttp.Handler())

	metrics := &http.Server{ //nolint:exhaustruct
		Addr:              fmt.Sprintf(":%d", config.MetricsServer.Port),
		Handler:           metricsHandler,
		ReadHeaderTimeout: time.Second,
	}

	// --------------------------------------------- Probes --------------------------------------------------------- //

	probesHandler := http.NewServeMux()

	probesHandler.Handle(config.ProbesServer.LivenessPath, http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

	probesHandler.Handle(config.ProbesServer.ReadinessPath, http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

	probes := &http.Server{ //nolint:exhaustruct
		Addr:              fmt.Sprintf(":%d", config.ProbesServer.Port),
		Handler:           probesHandler,
		ReadHeaderTimeout: time.Second,
	}

	// --------------------------------------------- Start Manager -------------------------------------------------- //

	// Start the controller-runtime manager in a goroutine.
	// The manager starts the cache (informers) which is required for the client to read objects.
	go func() {
		slog.Info("Starting controller-runtime manager (cache)")
		if err := mgr.Start(ctx); err != nil {
			slog.ErrorContext(ctx, "controller-runtime manager stopped with error", "error", err.Error())
			gs.Shutdown(1)
		}
	}()

	// Pre-register informers for Profile and Assignment types.
	// This ensures the informers are created before we wait for cache sync.
	// Without this, the dynamic cache client creates informers on-demand when first used,
	// which can cause the first requests to hang waiting for the informer to sync.
	slog.Info("Pre-registering informers for Profile and Assignment...")
	cache := mgr.GetCache()
	if _, err := cache.GetInformer(ctx, &v1alpha1.Profile{}); err != nil {
		slog.ErrorContext(ctx, "failed to get Profile informer", "error", err.Error())
		gs.Shutdown(1)
	}
	if _, err := cache.GetInformer(ctx, &v1alpha1.Assignment{}); err != nil {
		slog.ErrorContext(ctx, "failed to get Assignment informer", "error", err.Error())
		gs.Shutdown(1)
	}

	// Wait for cache to be synced before starting HTTP servers
	slog.Info("Waiting for cache to sync...")
	if !cache.WaitForCacheSync(ctx) {
		slog.ErrorContext(ctx, "failed to wait for cache sync")
		gs.Shutdown(1)
	}
	slog.Info("Cache synced successfully")

	// --------------------------------------------- Run Server ----------------------------------------------------- //

	httputil.Serve(map[string]*http.Server{
		"shaper":  shaperServer,
		"metrics": metrics,
		"probes":  probes,
	}, gs)

	slog.Info("✅ gracefully stopped", "binary", Name)
}
