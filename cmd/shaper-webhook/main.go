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
	"os"
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
	_, _ = fmt.Fprintf(os.Stdout, "Starting %s version %s (%s) %s\n", Name, Version, CommitSHA, BuildTimestamp)

	// TODO: implement me
	panic("implement me")
}
