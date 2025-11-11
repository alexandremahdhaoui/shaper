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

// Package v1alpha1 contains API Schema definitions for the v1alpha1 API group
// +kubebuilder:object:generate=true
// +groupName=shaper.amahdha.com
package v1alpha1

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

const (
	Group   = "shaper.amahdha.com"
	Version = "v1alpha1"

	UUIDPrefix      = "uuid"
	BuildarchPrefix = "buildarch"
)

var (
	GroupVersion  = schema.GroupVersion{Group: Group, Version: Version}
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
	AddToScheme   = SchemeBuilder.AddToScheme
)

// LabelSelector returns a label selector for the given key and prefixes.
//
// The label selector is in the format: `<prefix>.<prefix>...<group>/<key>`.
// If no prefixes are provided, the format is: `<group>/<key>`.
func LabelSelector(key string, prefixes ...string) string {
	label := fmt.Sprintf("%s/%s", Group, key)

	if len(prefixes) > 0 {
		label = fmt.Sprintf("%s.%s", strings.Join(prefixes, "."), label)
	}

	return label
}

// NewUUIDLabelSelector returns a new UUID label selector.
func NewUUIDLabelSelector(id uuid.UUID) string {
	return LabelSelector(id.String(), UUIDPrefix)
}

// SetUUIDLabelSelector sets a UUID label selector on a client.Object.
func SetUUIDLabelSelector(obj client.Object, id uuid.UUID, value string) {
	obj.GetLabels()[NewUUIDLabelSelector(id)] = value
}

// IsUUIDLabelSelector returns true if the given key is a UUID label selector.
func IsUUIDLabelSelector(key string) bool {
	return strings.Contains(key, LabelSelector("", UUIDPrefix))
}

// IsInternalLabel returns true if the given key is an internal label.
func IsInternalLabel(key string) bool {
	return strings.Contains(key, Group)
}

// UUIDLabelSelectors returns a map of UUIDs to names and a reverse map of names to UUIDs.
func UUIDLabelSelectors(labels map[string]string) (idNameMap map[uuid.UUID]string, reverse map[string]uuid.UUID, err error) {
	idNameMap = make(map[uuid.UUID]string)
	reverse = make(map[string]uuid.UUID)
	for k, v := range labels {
		if !IsUUIDLabelSelector(k) {
			continue
		}

		id, err := uuid.Parse(strings.TrimPrefix(k, LabelSelector("", UUIDPrefix)))
		if err != nil {
			return nil, nil, err // TODO: wrap err
		}

		idNameMap[id] = v
		reverse[v] = id // Fixed: reverse map should map content name (v) to UUID, not label key (k) to UUID
	}

	return idNameMap, reverse, nil
}
