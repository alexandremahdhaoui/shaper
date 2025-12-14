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

// Package network provides managers for Linux networking components.
//
// The package includes three main managers:
//
//   - BridgeManager: Manages Linux network bridges
//   - DnsmasqManager: Manages dnsmasq DHCP/TFTP server processes
//   - LibvirtNetworkManager: Manages libvirt virtual networks
//
// # Manager Pattern
//
// All managers follow a consistent pattern:
//   - Constructor injection of dependencies (execcontext.Context or libvirt connection)
//   - Create/Get/Delete methods that accept context.Context
//   - Idempotent Create and Delete operations
//   - Error-based existence checking (Get returns ErrXxxNotFound)
//
// # Example Usage
//
//	import (
//	    "context"
//	    "errors"
//	    "github.com/alexandremahdhaoui/shaper/pkg/network"
//	    "github.com/alexandremahdhaoui/shaper/pkg/execcontext"
//	)
//
//	// Create a bridge with sudo
//	execCtx := execcontext.New(nil, []string{"sudo"})
//	mgr := network.NewBridgeManager(execCtx)
//	ctx := context.Background()
//
//	err := mgr.Create(ctx, network.BridgeConfig{
//	    Name: "br0",
//	    CIDR: "192.168.100.1/24",
//	})
//	if err != nil {
//	    // handle error
//	}
//
//	// Check if bridge exists
//	info, err := mgr.Get(ctx, "br0")
//	if errors.Is(err, network.ErrBridgeNotFound) {
//	    // bridge doesn't exist
//	}
//
//	// Delete bridge
//	err = mgr.Delete(ctx, "br0")
//
// # Execution Context
//
// BridgeManager and DnsmasqManager accept an execcontext.Context which allows
// prepending commands (e.g., "sudo") for elevated privileges:
//
//	// For tests requiring root
//	execCtx := execcontext.New(nil, []string{"sudo"})
//	mgr := network.NewBridgeManager(execCtx)
package network
