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
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/alexandremahdhaoui/shaper/internal/util/httputil"
	"k8s.io/client-go/dynamic"

	"github.com/alexandremahdhaoui/shaper/pkg/generated/shaperserver"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/alexandremahdhaoui/shaper/internal/adapter"
	"github.com/alexandremahdhaoui/shaper/internal/controller"
	"github.com/alexandremahdhaoui/shaper/internal/driver/server"
	"github.com/alexandremahdhaoui/shaper/internal/k8s"
	"github.com/alexandremahdhaoui/shaper/internal/types"
	"github.com/alexandremahdhaoui/shaper/internal/util/gracefulshutdown"
)

const (
	Name             = "shaper-api"
	ConfigPathEnvKey = "IPXER_CONFIG_PATH"
)

var (
	Version        = "dev" //nolint:gochecknoglobals // set by ldflags
	CommitSHA      = "n/a" //nolint:gochecknoglobals // set by ldflags
	BuildTimestamp = "n/a" //nolint:gochecknoglobals // set by ldflags
)

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

	cl, err := k8s.NewKubeClient(restConfig)
	if err != nil {
		slog.ErrorContext(ctx, "creating kube client", "error", err.Error())
		gs.Shutdown(1)
	}

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

	// --------------------------------------------- Run Server ----------------------------------------------------- //

	httputil.Serve(map[string]*http.Server{
		"shaper":   shaperServer,
		"metrics": metrics,
		"probes":  probes,
	}, gs)

	slog.Info("✅ gracefully stopped", "binary", Name)
}

