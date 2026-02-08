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

package reconciler

import (
	"context"
	"errors"

	"github.com/alexandremahdhaoui/shaper/pkg/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ProfileReconciler reconciles Profile objects
type ProfileReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

// Verify ProfileReconciler implements reconcile.Reconciler
var _ reconcile.Reconciler = &ProfileReconciler{}

// Reconcile implements the reconciliation loop for Profile resources
// It ensures UUIDs for exposed additional content are consistent between Labels and Status.
// Labels are used for fast lookup via label selectors (by the Profile adapter).
// Status is used for stable UUID reference (by controllers and E2E tests).
//
// The reconciler coordinates with the Profile webhook:
// - If webhook set labels: reconciler copies UUIDs from labels to status
// - If webhook didn't run: reconciler generates UUIDs and sets both labels and status
func (r *ProfileReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("profile", req.NamespacedName)

	// Fetch the Profile
	var profile v1alpha1.Profile
	if err := r.Get(ctx, req.NamespacedName, &profile); err != nil {
		if apierrors.IsNotFound(err) {
			// Profile was deleted, nothing to do
			log.V(1).Info("Profile not found, likely deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Join(err, errors.New("failed to get profile"))
	}

	// Parse existing UUID labels (may have been set by webhook)
	_, nameToUUID, err := v1alpha1.UUIDLabelSelectors(profile.Labels)
	if err != nil {
		log.Error(err, "Failed to parse UUID labels")
		return ctrl.Result{}, errors.Join(err, errors.New("failed to parse uuid labels"))
	}

	// Initialize maps if needed
	needsStatusUpdate := false
	needsLabelUpdate := false

	if profile.Status.ExposedAdditionalContent == nil {
		profile.Status.ExposedAdditionalContent = make(map[string]string)
		needsStatusUpdate = true
	}
	if profile.Labels == nil {
		profile.Labels = make(map[string]string)
	}

	// Ensure UUIDs are consistent between Labels and Status for each exposed content
	for _, content := range profile.Spec.AdditionalContent {
		if !content.Exposed {
			continue
		}

		statusUUID := profile.Status.ExposedAdditionalContent[content.Name]
		labelUUID, hasLabelUUID := nameToUUID[content.Name]

		if hasLabelUUID && statusUUID == "" {
			// Label exists (from webhook), copy to status
			profile.Status.ExposedAdditionalContent[content.Name] = labelUUID.String()
			needsStatusUpdate = true
			log.Info("Copied UUID from label to status",
				"contentName", content.Name,
				"uuid", labelUUID.String())
		} else if !hasLabelUUID && statusUUID == "" {
			// Neither exists, generate new UUID and set both
			contentUUID := uuid.New()
			profile.Status.ExposedAdditionalContent[content.Name] = contentUUID.String()
			profile.Labels[v1alpha1.NewUUIDLabelSelector(contentUUID)] = content.Name
			needsStatusUpdate = true
			needsLabelUpdate = true
			log.Info("Generated new UUID for exposed content",
				"contentName", content.Name,
				"uuid", contentUUID.String())
		} else if hasLabelUUID && statusUUID != "" && statusUUID != labelUUID.String() {
			// Both exist but mismatch - prefer label (was set earlier by webhook)
			profile.Status.ExposedAdditionalContent[content.Name] = labelUUID.String()
			needsStatusUpdate = true
			log.Info("Resolved UUID mismatch (using label UUID)",
				"contentName", content.Name,
				"labelUUID", labelUUID.String(),
				"statusUUID", statusUUID)
		} else if !hasLabelUUID && statusUUID != "" {
			// Status exists but label doesn't - add label for fast lookup
			parsedUUID, parseErr := uuid.Parse(statusUUID)
			if parseErr == nil {
				profile.Labels[v1alpha1.NewUUIDLabelSelector(parsedUUID)] = content.Name
				needsLabelUpdate = true
				log.Info("Added missing label from status UUID",
					"contentName", content.Name,
					"uuid", statusUUID)
			}
		}
		// else: both exist and match, nothing to do
	}

	// Update labels if needed (must happen before status update)
	if needsLabelUpdate {
		// Preserve the status updates before updating labels
		statusSnapshot := profile.Status.ExposedAdditionalContent

		if err := r.Update(ctx, &profile); err != nil {
			log.Error(err, "Failed to update Profile labels")
			return ctrl.Result{}, errors.Join(err, errors.New("failed to update profile labels"))
		}
		log.Info("Successfully updated Profile labels")

		// Re-fetch profile after label update to get the new resourceVersion
		// This is required before updating status, otherwise the status update will fail
		if err := r.Get(ctx, req.NamespacedName, &profile); err != nil {
			log.Error(err, "Failed to re-fetch Profile after label update")
			return ctrl.Result{}, errors.Join(err, errors.New("failed to re-fetch profile"))
		}

		// Restore the status updates we calculated
		profile.Status.ExposedAdditionalContent = statusSnapshot
	}

	// Update status if needed (idempotent)
	if needsStatusUpdate {
		if err := r.Status().Update(ctx, &profile); err != nil {
			log.Error(err, "Failed to update Profile status")
			return ctrl.Result{}, errors.Join(err, errors.New("failed to update profile status"))
		}
		log.Info("Successfully updated Profile status")
	}

	if !needsStatusUpdate && !needsLabelUpdate {
		log.V(1).Info("No update needed")
	}

	return ctrl.Result{}, nil
}
