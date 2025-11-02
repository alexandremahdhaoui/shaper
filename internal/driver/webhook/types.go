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

package webhook

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
)

type validatingFunc = func(ctx context.Context, obj runtime.Object) error

// NewUnsupportedResource returns a new error for an unsupported resource.
func NewUnsupportedResource(obj runtime.Object, errs ...error) error {
	return errors.Join(
		errors.Join(errs...),
		fmt.Errorf("webhook does not support resource with GroupVersionKind=\"%#v\"",
			obj.GetObjectKind().GroupVersionKind()),
	)
}
