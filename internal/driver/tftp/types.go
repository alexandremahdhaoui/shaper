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

package tftp

// ServerConfig holds the configuration for the TFTP server
type ServerConfig struct {
	// Address is the address to bind to (e.g., ":69" for default TFTP port)
	Address string

	// RootDir is the directory from which files will be served
	RootDir string

	// ReadOnly enables read-only mode (disables write operations)
	ReadOnly bool

	// Timeout is the timeout for TFTP operations in seconds
	Timeout int

	// Retries is the number of retries for failed operations
	Retries int
}

// NewDefaultConfig returns a ServerConfig with sensible defaults
func NewDefaultConfig() *ServerConfig {
	return &ServerConfig{
		Address:  ":69",
		RootDir:  "/var/lib/shaper/tftp",
		ReadOnly: true,
		Timeout:  5,
		Retries:  5,
	}
}
