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

package cloudinit

import (
	"fmt"
	"os"
	"strings"

	"sigs.k8s.io/yaml"
)

type User struct {
	Name              string   `json:"name"`
	Sudo              string   `json:"sudo"`
	Shell             string   `json:"shell"`
	HomeDir           string   `json:"homedir,omitempty"`
	SSHAuthorizedKeys []string `json:"ssh_authorized_keys"`
	SSHKeys           *SSHKeys `json:"ssh_keys,omitempty"`
	SSHDeleteKeys     bool     `json:"ssh_deletekeys,omitempty"`
}

func NewUser(name string, publicKeyPathList ...string) (User, error) {
	authorizedKeys := make([]string, 0, len(publicKeyPathList))
	for _, path := range publicKeyPathList {
		b, err := os.ReadFile(path)
		if err != nil {
			return User{}, fmt.Errorf("ERROR: Failed to read file: %v", err)
		}
		authorizedKeys = append(authorizedKeys, string(b))
	}
	return User{
		Name:              name,
		Sudo:              "ALL=(ALL) NOPASSWD:ALL",
		Shell:             "/bin/bash",
		SSHAuthorizedKeys: authorizedKeys,
	}, nil
}

func NewUserWithAuthorizedKeys(name string, authorizedKeys []string) User {
	return User{
		Name:              name,
		Sudo:              "ALL=(ALL) NOPASSWD:ALL",
		Shell:             "/bin/bash",
		SSHAuthorizedKeys: authorizedKeys,
	}
}

type SSHKeys struct {
	RSAPrivate string `json:"rsa_private"`
	RSAPublic  string `json:"rsa_public"`
}

type WriteFile struct {
	Path        string `json:"path"`
	Permissions string `json:"permissions,omitempty"`
	Content     string `json:"content"`
}

type UserData struct {
	Hostname      string      `json:"hostname"`
	PackageUpdate bool        `json:"package_update,omitempty"`
	Packages      []string    `json:"packages,omitempty"`
	Users         []User      `json:"users"`
	WriteFiles    []WriteFile `json:"write_files,omitempty"`
	RunCommands   []string    `json:"runcmd,omitempty"`
}

func (ud UserData) Render() (string, error) {
	b, err := yaml.Marshal(ud)
	if err != nil {
		return "", fmt.Errorf("cannot render cloud-config from UserData: %v", err)
	}
	return fmt.Sprintf("#cloud-config\n%s", string(b)), nil
}

func NewRSAKeyFromPrivateKeyFile(privateKeyPath string) (SSHKeys, error) {
	privateKey, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return SSHKeys{}, fmt.Errorf("cannot read SSH private key at %s", privateKeyPath)
	}

	// bit hacky
	publicKeyPath := privateKeyPath + ".pub"
	if _, err := os.Stat(publicKeyPath); os.IsNotExist(err) {
		return SSHKeys{}, fmt.Errorf("SSH public key not found at %s", publicKeyPath)
	}

	publicKey, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return SSHKeys{}, fmt.Errorf("failed to read SSH public key: %w", err)
	}

	return SSHKeys{
		RSAPrivate: strings.TrimSpace(string(privateKey)),
		RSAPublic:  strings.TrimSpace(string(publicKey)),
	}, nil
}
