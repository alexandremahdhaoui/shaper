//go:build e2e

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

package e2e

import (
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrVMNotFound indicates the VM was not found in libvirt.
	ErrVMNotFound = errors.New("VM not found")
	// ErrVMUUIDParse indicates a failure to parse the VM UUID.
	ErrVMUUIDParse = errors.New("failed to parse VM UUID")
	// ErrVMStateTimeout indicates the VM did not reach the expected state within the timeout.
	ErrVMStateTimeout = errors.New("VM state timeout")
	// ErrVirshCommand indicates a failure to execute a virsh command.
	ErrVirshCommand = errors.New("virsh command failed")
)

// GetVMUUID returns the UUID of a VM by name using virsh domuuid.
func GetVMUUID(vmName string) (uuid.UUID, error) {
	cmd := exec.Command("virsh", "domuuid", vmName)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// Check if VM not found in error message
			if strings.Contains(string(exitErr.Stderr), "Domain not found") ||
				strings.Contains(string(exitErr.Stderr), "failed to get domain") {
				return uuid.Nil, errors.Join(ErrVMNotFound, err)
			}
		}
		return uuid.Nil, errors.Join(ErrVirshCommand, err)
	}

	// Parse the UUID from output (format: "c4a94672-05a1-4eda-a186-b4aa4544b146\n")
	uuidStr := strings.TrimSpace(string(output))
	id, err := uuid.Parse(uuidStr)
	if err != nil {
		return uuid.Nil, errors.Join(ErrVMUUIDParse, err)
	}

	return id, nil
}

// GetVMState returns the current state of a VM using virsh domstate.
func GetVMState(vmName string) (string, error) {
	cmd := exec.Command("virsh", "domstate", vmName)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if strings.Contains(string(exitErr.Stderr), "Domain not found") ||
				strings.Contains(string(exitErr.Stderr), "failed to get domain") {
				return "", errors.Join(ErrVMNotFound, err)
			}
		}
		return "", errors.Join(ErrVirshCommand, err)
	}

	return strings.TrimSpace(string(output)), nil
}

// WaitForVMState waits for a VM to reach the specified state within the given timeout.
// Common states are: "running", "shut off", "paused", "idle", "crashed".
func WaitForVMState(vmName, expectedState string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	pollInterval := 500 * time.Millisecond

	for time.Now().Before(deadline) {
		currentState, err := GetVMState(vmName)
		if err != nil {
			// If VM not found and we're waiting for "shut off", that's valid
			if errors.Is(err, ErrVMNotFound) && expectedState == "shut off" {
				return nil
			}
			return err
		}

		if currentState == expectedState {
			return nil
		}

		time.Sleep(pollInterval)
	}

	return errors.Join(ErrVMStateTimeout,
		errors.New("expected state: "+expectedState+", timeout: "+timeout.String()))
}

// StartVM starts a VM using virsh start.
func StartVM(vmName string) error {
	cmd := exec.Command("virsh", "start", vmName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Join(ErrVirshCommand, errors.New(string(output)), err)
	}
	return nil
}

// StopVM stops a VM using virsh destroy (force stop).
func StopVM(vmName string) error {
	cmd := exec.Command("virsh", "destroy", vmName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Ignore error if VM is already shut off
		if strings.Contains(string(output), "domain is not running") {
			return nil
		}
		return errors.Join(ErrVirshCommand, errors.New(string(output)), err)
	}
	return nil
}

// ShutdownVM gracefully shuts down a VM using virsh shutdown.
func ShutdownVM(vmName string) error {
	cmd := exec.Command("virsh", "shutdown", vmName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Join(ErrVirshCommand, errors.New(string(output)), err)
	}
	return nil
}

// IsVMRunning checks if a VM is currently running.
func IsVMRunning(vmName string) (bool, error) {
	state, err := GetVMState(vmName)
	if err != nil {
		if errors.Is(err, ErrVMNotFound) {
			return false, nil
		}
		return false, err
	}
	return state == "running", nil
}
