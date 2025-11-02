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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVMConfig_Instantiation(t *testing.T) {
	config := &VMConfig{
		Name:        "test-vm",
		ImagePath:   "/tmp/test.img",
		MemoryMB:    2048,
		VCPUs:       2,
		NetworkMode: "bridge",
		BridgeName:  "virbr0",
		MACAddress:  "52:54:00:12:34:56",
		BootOrder:   []string{"network", "hd"},
		TempDir:     "/tmp/test",
	}

	assert.Equal(t, "test-vm", config.Name)
	assert.Equal(t, "/tmp/test.img", config.ImagePath)
	assert.Equal(t, 2048, config.MemoryMB)
	assert.Equal(t, 2, config.VCPUs)
	assert.Equal(t, "bridge", config.NetworkMode)
	assert.Equal(t, "virbr0", config.BridgeName)
	assert.Equal(t, "52:54:00:12:34:56", config.MACAddress)
	assert.Equal(t, []string{"network", "hd"}, config.BootOrder)
	assert.Equal(t, "/tmp/test", config.TempDir)
}

func TestVMMetadata_Instantiation(t *testing.T) {
	metadata := &VMMetadata{
		Name:         "test-vm",
		UUID:         "550e8400-e29b-41d4-a716-446655440000",
		IP:           "192.168.122.10",
		MACAddress:   "52:54:00:12:34:56",
		State:        "running",
		CreatedFiles: []string{"/tmp/test1", "/tmp/test2"},
	}

	assert.Equal(t, "test-vm", metadata.Name)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", metadata.UUID)
	assert.Equal(t, "192.168.122.10", metadata.IP)
	assert.Equal(t, "52:54:00:12:34:56", metadata.MACAddress)
	assert.Equal(t, "running", metadata.State)
	assert.Len(t, metadata.CreatedFiles, 2)
}

func TestVMConfig_DefaultValues(t *testing.T) {
	config := &VMConfig{}

	assert.Empty(t, config.Name)
	assert.Empty(t, config.ImagePath)
	assert.Equal(t, 0, config.MemoryMB)
	assert.Equal(t, 0, config.VCPUs)
	assert.Nil(t, config.BootOrder)
}
