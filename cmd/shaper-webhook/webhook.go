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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	driverwebhook "github.com/alexandremahdhaoui/shaper/internal/driver/webhook"
	"github.com/alexandremahdhaoui/shaper/pkg/v1alpha1"
)

// setupWebhookServer registers webhooks with the controller-runtime manager.
func setupWebhookServer(
	mgr ctrl.Manager,
	assignmentWebhook *driverwebhook.Assignment,
	profileWebhook *driverwebhook.Profile,
) error {
	server := mgr.GetWebhookServer()

	// Register Assignment validation webhook
	server.Register(
		"/validate-shaper-amahdha-com-v1alpha1-assignment",
		&webhook.Admission{
			Handler: admission.WithCustomValidator(
				mgr.GetScheme(),
				&v1alpha1.Assignment{},
				assignmentWebhook,
			),
		},
	)

	// Register Assignment mutation webhook
	server.Register(
		"/mutate-shaper-amahdha-com-v1alpha1-assignment",
		&webhook.Admission{
			Handler: admission.WithCustomDefaulter(
				mgr.GetScheme(),
				&v1alpha1.Assignment{},
				assignmentWebhook,
			),
		},
	)

	// Register Profile validation webhook
	server.Register(
		"/validate-shaper-amahdha-com-v1alpha1-profile",
		&webhook.Admission{
			Handler: admission.WithCustomValidator(
				mgr.GetScheme(),
				&v1alpha1.Profile{},
				profileWebhook,
			),
		},
	)

	// Register Profile mutation webhook
	server.Register(
		"/mutate-shaper-amahdha-com-v1alpha1-profile",
		&webhook.Admission{
			Handler: admission.WithCustomDefaulter(
				mgr.GetScheme(),
				&v1alpha1.Profile{},
				profileWebhook,
			),
		},
	)

	return nil
}
