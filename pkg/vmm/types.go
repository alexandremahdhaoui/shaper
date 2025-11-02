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

package vmm

import "context"

// VMConfig contains configuration for creating a VM
type VMConfig struct {
	Name        string
	ImagePath   string
	MemoryMB    int
	VCPUs       int
	NetworkMode string // "bridge", "nat", "user"
	BridgeName  string // for bridge mode
	MACAddress  string // optional, auto-generated if empty
	BootOrder   []string // e.g., ["network", "hd"]
	UserData    interface{} // cloud-init data
	TempDir     string
}

// VMMetadata contains runtime information about a VM
type VMMetadata struct {
	Name         string
	UUID         string
	IP           string
	MACAddress   string
	State        string // "running", "stopped", "paused"
	CreatedFiles []string // for cleanup tracking
}

// VMM interface for VM lifecycle management
type VMM interface {
	CreateVM(config *VMConfig) (*VMMetadata, error)
	DestroyVM(ctx context.Context, name string) error
	DomainExists(ctx context.Context, name string) (bool, error)
	GetVMIP(ctx context.Context, name string) (string, error)
	Close() error
}
