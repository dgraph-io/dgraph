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
	"github.com/hashicorp/vault/api"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type vaultKeyReader struct {
	Addr     string
	RoleID   string
	SecretID string
	Path     string
	Field    string
}

// RegisterVaultFlags registers the required flags to integrate with Vault.
func RegisterFlags(flag *pflag.FlagSet) {
	// The following are Vault options. Applicable for alpha, live, bulk, debug, restore sub-cmds
	flag.String("vault_addr", "http://localhost:8200",
		"Vault server's address in the form http://ip:port.")
	flag.String("vault_roleID", "",
		"Vault role-id used for approle auth.")
	flag.String("vault_secretID", "",
		"Vault secret-id used for approle auth.")
	flag.String("vault_path", "dgraph",
		"Vault kv store path.")
	flag.String("vault_field", "enc_key",
		"Vault kv store field whose value is the encryption key.")
}

func NewVaultKeyReader(cfg *viper.Viper) (*vaultKeyReader, error) {
	v := &vaultKeyReader{
		Addr:     cfg.GetString("vault_addr"),
		RoleID:   cfg.GetString("vault_roleID"),
		SecretID: cfg.GetString("vault_secretID"),
		Path:     cfg.GetString("vault_path"),
		Field:    cfg.GetString("vault_field"),
	}

	if v.RoleID != "" && v.SecretID != "" {
		return v, nil
	}
	return nil, errors.Errorf("vault_roleID and vault_secretID must both be specified")
}

func (vKR *vaultKeyReader) ReadKey() ([]byte, error) {
	// Get a Vault Client.
	vConfig := &api.Config{
		Address: vKR.Addr,
	}
	client, err := api.NewClient(vConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "while creating a new vault client")
	}

	// Login into the Vault with approle Auth.
	data := map[string]interface{}{
		"role_id":   vKR.RoleID,
		"secret_id": vKR.SecretID,
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
	secret, err := client.Logical().Read("secret/data/" + vKR.Path)
	if err != nil {
		return nil, errors.Wrapf(err, "while reading key from kv store at %v", vKR.Path)
	}

	// Parse key from response
	m, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		return nil, errors.Errorf("kv store read response from vault is bad")
	}
	kVal, ok := m[vKR.Field]
	if !ok {
		return nil, errors.Errorf("secret key not found at %v", vKR.Field)
	}
	kbyte := []byte(kVal.(string))

	// Validate key length suitable for AES
	klen := len(kbyte)
	if klen != 16 && klen != 32 && klen != 64 {
		return nil, errors.Errorf("bad key length %v from vault", klen)
	}
	return kbyte, nil
}
