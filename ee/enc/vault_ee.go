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

package enc

import (
	"io/ioutil"

	"github.com/dgraph-io/dgraph/x"
	"github.com/hashicorp/vault/api"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Configuration options of Vault.
const (
	vaultAddr         = "vault_addr"
	vaultRoleIDFile   = "vault_roleid_file"
	vaultSecretIDFile = "vault_secretid_file"
	vaultPath         = "vault_path"
	vaultField        = "vault_field"
)

// RegisterVaultFlags registers the required flags to integrate with Vault.
func registerVaultFlags(flag *pflag.FlagSet) {
	// The following are Vault options. Applicable for alpha, live, bulk, debug, restore sub-cmds
	flag.String(vaultAddr, "http://localhost:8200",
		"Vault server's address in the form http://ip:port.")
	flag.String(vaultRoleIDFile, "",
		"File containing Vault role-id used for approle auth.")
	flag.String(vaultSecretIDFile, "",
		"File containing Vault secret-id used for approle auth.")
	flag.String(vaultPath, "dgraph",
		"Vault kv store path.")
	flag.String(vaultField, "enc_key",
		"Vault kv store field whose value is the encryption key.")
}

// vaultKeyReader implements the KeyReader interface. It reads the key from vault server.
type vaultKeyReader struct {
	addr     string
	roleID   string
	secretID string
	path     string
	field    string
}

func newVaultKeyReader(cfg *viper.Viper) (*vaultKeyReader, error) {
	v := &vaultKeyReader{
		addr:     cfg.GetString(vaultAddr),
		roleID:   cfg.GetString(vaultRoleIDFile),
		secretID: cfg.GetString(vaultSecretIDFile),
		path:     cfg.GetString(vaultPath),
		field:    cfg.GetString(vaultField),
	}

	if v.addr == "" || v.path == "" || v.field == "" {
		return nil, errors.Errorf("%v, %v or %v is missing", vaultAddr, vaultPath, vaultField)
	}

	if v.roleID != "" && v.secretID != "" {
		return v, nil
	}
	return nil, errors.Errorf("%v and %v must both be specified",
		vaultRoleIDFile, vaultSecretIDFile)
}

// ReadKey reads the key from the vault kv store.
func (vkr *vaultKeyReader) ReadKey() (x.SensitiveByteSlice, error) {
	if vkr == nil {
		return nil, errors.Errorf("nil vaultKeyReader")
	}

	// Read the files.
	roleID, err := ioutil.ReadFile(vkr.roleID)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading role-id file (%v)", vkr.roleID)
	}
	secretID, err := ioutil.ReadFile(vkr.secretID)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading secret-id file (%v)", vkr.secretID)
	}

	// Get a Vault Client.
	vConfig := &api.Config{
		Address: vkr.addr,
	}
	client, err := api.NewClient(vConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "while creating a new vault client")
	}

	// Login into the Vault with approle Auth.
	data := map[string]interface{}{
		"role_id":   string(roleID),
		"secret_id": string(secretID),
	}
	resp, err := client.Logical().Write("auth/approle/login", data)
	if err != nil {
		return nil, errors.Wrapf(err, "while logging into vault")
	}
	if resp.Auth == nil {
		return nil, errors.Errorf("login response from vault is bad")
	}
	client.SetToken(resp.Auth.ClientToken)

	// Read from KV store
	secret, err := client.Logical().Read("secret/data/" + vkr.path)
	if err != nil || secret == nil {
		return nil, errors.Errorf("error or nil secret on reading key at %v: "+
			"err %v", vkr.path, err)
	}

	// Parse key from response
	m, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		return nil, errors.Errorf("kv store read response from vault is bad")
	}
	kVal, ok := m[vkr.field]
	if !ok {
		return nil, errors.Errorf("secret key not found at %v", vkr.field)
	}
	kbyte := []byte(kVal.(string))

	// Validate key length suitable for AES
	klen := len(kbyte)
	if klen != 16 && klen != 32 && klen != 64 {
		return nil, errors.Errorf("bad key length %v from vault", klen)
	}
	return kbyte, nil
}
