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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&Assignment{}, &AssignmentList{})
}

var (
	// DefaultAssignmentLabel is used to query default assignments.
	DefaultAssignmentLabel = LabelSelector("default-assignment")

	// BuildarchList Label Selector

	// I386BuildarchLabelSelector is the label selector for the i386 build architecture.
	I386BuildarchLabelSelector = LabelSelector(I386.String(), BuildarchPrefix)
	// X8664BuildarchLabelSelector is the label selector for the x86_64 build architecture.
	X8664BuildarchLabelSelector = LabelSelector(X8664.String(), BuildarchPrefix)
	// Arm32BuildarchLabelSelector is the label selector for the arm32 build architecture.
	Arm32BuildarchLabelSelector = LabelSelector(Arm32.String(), BuildarchPrefix)
	// Arm64BuildarchLabelSelector is the label selector for the arm64 build architecture.
	Arm64BuildarchLabelSelector = LabelSelector(Arm64.String(), BuildarchPrefix)

	// datastructures

	// AllowedBuildarchList is a list of allowed build architectures.
	AllowedBuildarchList = []Buildarch{Arm32, Arm64, I386, X8664}
	// AllowedBuildarch is a map of allowed build architectures.
	AllowedBuildarch = func() map[Buildarch]any {
		out := make(map[Buildarch]any)

		for _, b := range AllowedBuildarchList {
			out[b] = nil
		}

		return out
	}()

	buildarchToLabel = map[Buildarch]string{
		Arm32: Arm64BuildarchLabelSelector,
		Arm64: Arm64BuildarchLabelSelector,
		I386:  I386BuildarchLabelSelector,
		X8664: X8664BuildarchLabelSelector,
	}
)

// Buildarch is the build architecture of the machine.
type Buildarch string

// String returns the string representation of the Buildarch.
func (b Buildarch) String() string {
	return string(b)
}

const (
	// BuildarchList

	// I386 - i386	32-bit x86 CPU
	I386 Buildarch = "i386"
	// X8664 - x86_64	64-bit x86 CPU
	X8664 Buildarch = "x86_64"
	// Arm32 - arm32	32-bit ARM CPU
	Arm32 Buildarch = "arm32"
	// Arm64 - arm64	64-bit ARM CPU
	Arm64 Buildarch = "arm64"
)

// apiVersion: shaper.amahdha.com/v1alpha1
// kind: Assignment
// metadata:
//   name: your-assignment
//   labels:
//     shaper.amahdha.com/buildarch: arm64
//     uuid.shaper.amahdha.com/c4a94672-05a1-4eda-a186-b4aa4544b146: ""
//     uuid.shaper.amahdha.com/3f5f3c39-584e-4c7c-b6ff-137e1aaa7175: ""
// spec:
//   # subjectSelectors map[string][]string
//   # the specified labels selects subjects that can iPXE boot the selected profile below.
//   subjectSelectors:
//     buildarch: # please note only 1 buildarch mat be specified at a time.
//       - arm64
//     serialNumber:
//       - c4a94672-05a1-4eda-a186-b4aa4544b146
//     uuid:
//       - 47c6da67-7477-4970-aa03-84e48ff4f6ad
//       - 3f5f3c39-584e-4c7c-b6ff-137e1aaa7175
//   # profileName string
//   profileName: 819f1859-a669-410b-adfc-d0bc128e2d7a
// status:
//   conditions: []

type (
	//+kubebuilder:object:root=true
	//+kubebuilder:subresources:status

	// Assignment is the Schema for the assignments API
	Assignment struct {
		metav1.TypeMeta   `json:",inline"`
		metav1.ObjectMeta `json:"metadata,omitempty"`

		Spec   AssignmentSpec   `json:"spec,omitempty"`
		Status AssignmentStatus `json:"status,omitempty"`
	}

	//+kubebuilder:object:root=true

	// AssignmentList contains a list of Assignment
	AssignmentList struct {
		metav1.TypeMeta `json:",inline"`
		metav1.ListMeta `json:"metadata,omitempty"`

		Items []Assignment `json:"items"`
	}

	// AssignmentSpec defines the desired state of Assignment
	AssignmentSpec struct {
		// SubjectSelectors is a map of selectors that are used to match a machine.
		SubjectSelectors SubjectSelectors `json:"subjectSelectors"`
		// ProfileName is the name of the profile to assign to the machine.
		ProfileName string `json:"profileName"`
		// IsDefault is true if this assignment is the default assignment.
		IsDefault bool `json:"isDefault"`
	}

	// AssignmentStatus defines the observed state of Assignment
	AssignmentStatus struct{}

	// SubjectSelectors is a map of selectors that are used to match a machine.
	SubjectSelectors struct {
		// BuildarchList is a list of build architectures to match.
		BuildarchList []Buildarch `json:"buildarch"`
		// UUIDList is a list of UUIDs to match.
		UUIDList []string `json:"uuidList"`
	}
)

// GetBuildarchList returns the list of build architectures for the assignment.
func (a *Assignment) GetBuildarchList() []Buildarch {
	out := make([]Buildarch, 0)

	if _, ok := a.Labels[Arm32BuildarchLabelSelector]; ok {
		out = append(out, Arm32)
	}

	if _, ok := a.Labels[Arm64BuildarchLabelSelector]; ok {
		out = append(out, Arm64)
	}

	if _, ok := a.Labels[I386BuildarchLabelSelector]; ok {
		out = append(out, I386)
	}

	if _, ok := a.Labels[X8664BuildarchLabelSelector]; ok {
		out = append(out, X8664)
	}

	return out
}

// SetBuildarch sets the build architecture for the assignment.
func (a *Assignment) SetBuildarch(buildarch Buildarch) {
	a.Labels[buildarchToLabel[buildarch]] = ""
}
