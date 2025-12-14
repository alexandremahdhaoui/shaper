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

package types

// Assignment is a struct that holds the name of an assignment and the name of the profile it assigns.
type Assignment struct {
	// Name is the name given to the Assignment resource itself.
	Name string
	// Namespace is the namespace of the Assignment resource.
	Namespace string
	// ProfileName is the name of the assigned profile.
	ProfileName string
	// SubjectSelectors contains the selectors used to match machines.
	SubjectSelectors map[string][]string
}
