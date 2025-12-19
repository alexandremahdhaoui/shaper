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

// AssignmentReconciler reconciles Assignment objects
type AssignmentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

// Verify AssignmentReconciler implements reconcile.Reconciler
var _ reconcile.Reconciler = &AssignmentReconciler{}

// Reconcile implements the reconciliation loop for Assignment resources
// It adds labels for UUID and buildarch from spec.subjectSelectors
func (r *AssignmentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("assignment", req.NamespacedName)

	// Fetch the Assignment
	var assignment v1alpha1.Assignment
	if err := r.Get(ctx, req.NamespacedName, &assignment); err != nil {
		if apierrors.IsNotFound(err) {
			// Assignment was deleted, nothing to do
			log.V(1).Info("Assignment not found, likely deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Join(err, errors.New("failed to get assignment"))
	}

	// Initialize labels if nil
	if assignment.Labels == nil {
		assignment.Labels = make(map[string]string)
	}

	needsUpdate := false

	// Add buildarch labels
	for _, buildarch := range assignment.Spec.SubjectSelectors.BuildarchList {
		labelKey := v1alpha1.LabelSelector(buildarch.String(), v1alpha1.BuildarchPrefix)
		if _, exists := assignment.Labels[labelKey]; !exists {
			assignment.Labels[labelKey] = ""
			needsUpdate = true
			log.Info("Added buildarch label",
				"buildarch", buildarch,
				"labelKey", labelKey)
		}
	}

	// Add UUID labels
	for _, uuidStr := range assignment.Spec.SubjectSelectors.UUIDList {
		// Parse UUID to validate format
		parsedUUID, err := uuid.Parse(uuidStr)
		if err != nil {
			log.Error(err, "Invalid UUID in subjectSelectors",
				"uuid", uuidStr)
			// Skip invalid UUIDs but don't fail the reconciliation
			continue
		}

		labelKey := v1alpha1.NewUUIDLabelSelector(parsedUUID)
		if _, exists := assignment.Labels[labelKey]; !exists {
			assignment.Labels[labelKey] = ""
			needsUpdate = true
			log.Info("Added UUID label",
				"uuid", uuidStr,
				"labelKey", labelKey)
		}
	}

	// Add default assignment label if isDefault is true
	if assignment.Spec.IsDefault {
		labelKey := v1alpha1.DefaultAssignmentLabel
		if _, exists := assignment.Labels[labelKey]; !exists {
			assignment.Labels[labelKey] = ""
			needsUpdate = true
			log.Info("Added default assignment label",
				"labelKey", labelKey)
		}
	}

	// Update if labels were added (idempotent)
	if needsUpdate {
		if err := r.Update(ctx, &assignment); err != nil {
			log.Error(err, "Failed to update Assignment labels")
			return ctrl.Result{}, errors.Join(err, errors.New("failed to update assignment labels"))
		}
		log.Info("Successfully updated Assignment labels")
	} else {
		log.V(1).Info("No label update needed")
	}

	return ctrl.Result{}, nil
}
