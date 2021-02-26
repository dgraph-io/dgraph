// +build !oss

/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package vault

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"reflect"

	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/golang/glog"
	"github.com/hashicorp/vault/api"
	"github.com/spf13/viper"
)

func GetKeys(config *viper.Viper) (aclKey, encKey x.SensitiveByteSlice) {
	// Avoid querying Vault unless the flag has been explicitly set.
	if !config.IsSet(flagVault) {
		return
	}

	vaultString := config.GetString(flagVault)
	vaultFlag := z.NewSuperFlag(vaultString)
	vaultConfig, err := parseFlags(vaultFlag)
	if err != nil {
		glog.Exit(err)
	}

	// Avoid querying Vault unless there is data we want to retrieve from Vault.
	if vaultConfig.aclField == "" && vaultConfig.encField == "" {
		return
	}

	client, err := newClient(vaultConfig.addr, vaultConfig.roleIdFile, vaultConfig.secretIdFile)
	if err != nil {
		glog.Exit(err)
	}

	kv, err := getKvStore(client, vaultConfig.path)
	if err != nil {
		glog.Exit(err)
	}

	if vaultConfig.aclField != "" {
		if aclKey, err = kv.getSensitiveBytes(vaultConfig.aclField, vaultConfig.aclFormat); err != nil {
			glog.Exit(err)
		}
	}
	if vaultConfig.encField != "" {
		if encKey, err = kv.getSensitiveBytes(vaultConfig.encField, vaultConfig.encFormat); err != nil {
			glog.Exit(err)
		}
	}

	return
}

// kvStore represents a KV store retrieved from the Vault KV Secrets Engine.
type kvStore map[string]interface{}

// getKvStore fetches a KV store from located at path.
func getKvStore(client *api.Client, path string) (kvStore, error) {
	secret, err := client.Logical().Read(path)
	if err != nil {
		return nil, fmt.Errorf("vault: error retrieving path %s: %s", path, err)
	}
	if secret == nil || secret.Data == nil {
		return nil, fmt.Errorf("vault: error retrieving path %s: empty response", path)
	}

	var kv kvStore
	kv, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		glog.Infof("vault: failed to parse response in KV V2 format, falling back to V1")
		kv = secret.Data
	}

	return kv, nil
}

// getSensitiveBytes retrieves a value from a kvStore, decoding it if necessary.
func (kv kvStore) getSensitiveBytes(field, format string) (x.SensitiveByteSlice, error) {
	value, ok := kv[field]
	if !ok {
		return nil, fmt.Errorf("vault: key '%s' not found", field)
	}
	valueString, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf(
			"vault: key '%s' is of type %s, expected string", field, reflect.TypeOf(value))
	}

	// Decode value if necessary.
	var valueBytes x.SensitiveByteSlice
	var err error
	if format == "base64" {
		valueBytes, err = base64.StdEncoding.DecodeString(valueString)
		if err != nil {
			return nil, fmt.Errorf(
				"vault: key '%s' could not be decoded as a base64 string: %s", field, err)
		}
	} else {
		valueBytes = x.SensitiveByteSlice(valueString)
	}

	return valueBytes, nil
}

// newClient creates an AppRole-authenticated Vault client using the provided credentials.
func newClient(address, roleIdPath, secretIdPath string) (*api.Client, error) {
	// Connect to Vault.
	client, err := api.NewClient(&api.Config{Address: address})
	if err != nil {
		return nil, fmt.Errorf("vault: error creating client: %s", err)
	}

	// Read Vault credentials from disk.
	roleId, err := ioutil.ReadFile(roleIdPath)
	if err != nil {
		return nil, fmt.Errorf("vault: error reading from role ID file: %s", err)
	}
	secretId, err := ioutil.ReadFile(secretIdPath)
	if err != nil {
		return nil, fmt.Errorf("vault: error reading from secret ID file: %s", err)
	}

	fmt.Printf("roleId: %s\n", roleId)
	fmt.Printf("secretId: %s\n", secretId)

	// Login into Vault with AppRole authentication.
	secret, err := client.Logical().Write("auth/approle/login", map[string]interface{}{
		"role_id":   roleId,
		"secret_id": secretId,
	})
	if err != nil {
		return nil, fmt.Errorf("vault: login error: %s", err)
	}
	if secret == nil || secret.Auth == nil {
		return nil, fmt.Errorf("vault: login error: empty response")
	}
	client.SetToken(secret.Auth.ClientToken)

	return client, nil
}
