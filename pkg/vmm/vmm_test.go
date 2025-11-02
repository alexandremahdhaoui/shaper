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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewVMM_WithDefaultOptions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	vmm, err := NewVMM()
	require.NoError(t, err)
	require.NotNil(t, vmm)

	// Verify it's actually a vmmImpl
	impl, ok := vmm.(*vmmImpl)
	require.True(t, ok)
	assert.NotNil(t, impl.conn)
	assert.Equal(t, "/tmp", impl.baseDir)

	// Clean up
	err = vmm.Close()
	assert.NoError(t, err)
}

func TestNewVMM_WithBaseDir(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	vmm, err := NewVMM(WithBaseDir("/custom/path"))
	require.NoError(t, err)
	require.NotNil(t, vmm)

	impl, ok := vmm.(*vmmImpl)
	require.True(t, ok)
	assert.Equal(t, "/custom/path", impl.baseDir)

	err = vmm.Close()
	assert.NoError(t, err)
}

func TestVMMInterface_ImplementsAllMethods(t *testing.T) {
	// This test verifies that vmmImpl implements all VMM interface methods
	// It's a compile-time check
	var _ VMM = (*vmmImpl)(nil)
}

func TestVMM_DomainExists_Unimplemented(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	vmm, err := NewVMM()
	require.NoError(t, err)
	defer vmm.Close()

	ctx := context.Background()
	exists, err := vmm.DomainExists(ctx, "non-existent-vm")
	require.NoError(t, err)
	assert.False(t, exists) // Should return false for unimplemented
}
