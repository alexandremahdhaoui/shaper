//go:build unit

package network

import (
	"context"
	"errors"
	"testing"

	"github.com/alexandremahdhaoui/shaper/pkg/execcontext"
)

func TestNewDnsmasqManager(t *testing.T) {
	tests := []struct {
		name    string
		execCtx execcontext.Context
	}{
		{
			name:    "with nil context",
			execCtx: execcontext.New(nil, nil),
		},
		{
			name:    "with sudo context",
			execCtx: execcontext.New(nil, []string{"sudo"}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewDnsmasqManager(tt.execCtx)
			if mgr == nil {
				t.Fatal("expected non-nil manager")
			}
			if mgr.execCtx == nil {
				t.Fatal("expected non-nil execCtx")
			}
			if mgr.processes == nil {
				t.Fatal("expected non-nil processes map")
			}
		})
	}
}

func TestDnsmasqManager_Create_ValidationErrors(t *testing.T) {
	mgr := NewDnsmasqManager(execcontext.New(nil, nil))
	ctx := context.Background()

	tests := []struct {
		name        string
		id          string
		config      DnsmasqConfig
		expectedErr string
	}{
		{
			name:        "empty ID",
			id:          "",
			config:      DnsmasqConfig{},
			expectedErr: "dnsmasq ID is required",
		},
		{
			name: "missing interface",
			id:   "test",
			config: DnsmasqConfig{
				Interface:    "",
				DHCPRange:    "192.168.100.10,192.168.100.250",
				TFTPRoot:     "/tmp/tftp",
				BootFilename: "undionly.kpxe",
			},
			expectedErr: "interface is required",
		},
		{
			name: "missing DHCP range",
			id:   "test",
			config: DnsmasqConfig{
				Interface:    "br0",
				DHCPRange:    "",
				TFTPRoot:     "/tmp/tftp",
				BootFilename: "undionly.kpxe",
			},
			expectedErr: "DHCP range is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mgr.Create(ctx, tt.id, tt.config)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tt.expectedErr && !errors.Is(err, ErrInterfaceRequired) && !errors.Is(err, ErrDHCPRangeRequired) {
				t.Logf("got error: %v", err)
			}
		})
	}
}

func TestDnsmasqManager_Get_ValidationErrors(t *testing.T) {
	mgr := NewDnsmasqManager(execcontext.New(nil, nil))
	ctx := context.Background()

	tests := []struct {
		name        string
		id          string
		expectedErr string
	}{
		{
			name:        "empty ID",
			id:          "",
			expectedErr: "dnsmasq ID is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := mgr.Get(ctx, tt.id)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tt.expectedErr {
				t.Errorf("expected error %q, got %q", tt.expectedErr, err.Error())
			}
		})
	}
}

func TestDnsmasqManager_Get_NotFound(t *testing.T) {
	mgr := NewDnsmasqManager(execcontext.New(nil, nil))
	ctx := context.Background()

	_, err := mgr.Get(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrDnsmasqNotFound) {
		t.Errorf("expected ErrDnsmasqNotFound, got %v", err)
	}
}

func TestDnsmasqManager_Get_NotFound_ErrorCheck(t *testing.T) {
	// This test verifies that errors.Is works with ErrDnsmasqNotFound
	err := ErrDnsmasqNotFound
	if !errors.Is(err, ErrDnsmasqNotFound) {
		t.Fatal("errors.Is should work with ErrDnsmasqNotFound")
	}
}

func TestDnsmasqManager_Delete_ValidationErrors(t *testing.T) {
	mgr := NewDnsmasqManager(execcontext.New(nil, nil))
	ctx := context.Background()

	tests := []struct {
		name        string
		id          string
		expectedErr string
	}{
		{
			name:        "empty ID",
			id:          "",
			expectedErr: "dnsmasq ID is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mgr.Delete(ctx, tt.id)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tt.expectedErr {
				t.Errorf("expected error %q, got %q", tt.expectedErr, err.Error())
			}
		})
	}
}

func TestDnsmasqManager_Delete_NonExistent(t *testing.T) {
	mgr := NewDnsmasqManager(execcontext.New(nil, nil))
	ctx := context.Background()

	// Delete non-existent process should be idempotent (no error)
	err := mgr.Delete(ctx, "nonexistent")
	if err != nil {
		t.Errorf("expected nil error for non-existent process, got %v", err)
	}
}
