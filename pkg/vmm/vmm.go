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

import (
	"context"

	"libvirt.org/go/libvirt"
)

// vmmImpl implements the VMM interface
type vmmImpl struct {
	conn    *libvirt.Connect
	baseDir string
}

// Option is a functional option for configuring vmmImpl
type Option func(*vmmImpl)

// WithBaseDir sets the base directory for VM artifacts
func WithBaseDir(dir string) Option {
	return func(v *vmmImpl) {
		v.baseDir = dir
	}
}

// WithConnection sets a custom libvirt connection URI
func WithConnection(uri string) Option {
	// TODO: implement custom URI support
	return func(v *vmmImpl) {}
}

// NewVMM creates a new VMM instance connected to qemu:///system by default
func NewVMM(opts ...Option) (VMM, error) {
	v := &vmmImpl{
		baseDir: "/tmp",
	}

	// Apply options
	for _, opt := range opts {
		opt(v)
	}

	// Connect to libvirt
	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		return nil, err
	}

	v.conn = conn
	return v, nil
}

// CreateVM creates and starts a new VM
func (v *vmmImpl) CreateVM(config *VMConfig) (*VMMetadata, error) {
	// TODO: implement VM creation
	return nil, nil
}

// DestroyVM destroys a VM by name
func (v *vmmImpl) DestroyVM(ctx context.Context, name string) error {
	// TODO: implement VM destruction
	return nil
}

// DomainExists checks if a VM exists
func (v *vmmImpl) DomainExists(ctx context.Context, name string) (bool, error) {
	// TODO: implement domain existence check
	return false, nil
}

// GetVMIP retrieves the IP address of a VM
func (v *vmmImpl) GetVMIP(ctx context.Context, name string) (string, error) {
	// TODO: implement IP retrieval
	return "", nil
}

// Close closes the libvirt connection
func (v *vmmImpl) Close() error {
	if v.conn != nil {
		_, err := v.conn.Close()
		return err
	}
	return nil
}
