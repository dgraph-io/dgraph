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

type Opts struct {
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

func SanityChecks(cfg *viper.Viper) error {
	v := Opts{
		Addr:     cfg.GetString("vault_addr"),
		RoleID:   cfg.GetString("vault_roleID"),
		SecretID: cfg.GetString("vault_secretID"),
		Path:     cfg.GetString("vault_path"),
		Field:    cfg.GetString("vault_field"),
	}

	if v.RoleID != "" && v.SecretID == "" ||
		v.RoleID == "" && v.SecretID != "" {
		return errors.Errorf("vault_roleID and vault_secretID must both be specified")
	}
	return nil
}

func ReadKey(cfg *viper.Viper) ([]byte, error) {
	v := Opts{
		Addr:     cfg.GetString("vault_addr"),
		RoleID:   cfg.GetString("vault_roleID"),
		SecretID: cfg.GetString("vault_secretID"),
		Path:     cfg.GetString("vault_path"),
		Field:    cfg.GetString("vault_field"),
	}

	// Get a Vault Client.
	vConfig := &api.Config{
		Address: v.Addr,
	}
	client, err := api.NewClient(vConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "while creating a new vault client")
	}

	// Login into the Vault with approle Auth.
	data := map[string]interface{}{
		"role_id":   v.RoleID,
		"secret_id": v.SecretID,
	}
	resp, err := client.Logical().Write("auth/approle/login", data)
	if err != nil {
		return nil, errors.Wrapf(err, "while logging into vault")
	}
	if resp.Auth == nil {
		return nil, errors.Wrapf(err, "login response from vault is bad")
	}
	client.SetToken(resp.Auth.ClientToken)

	// LifeTimeWatcher renewal logic
	//go RenewAuth(resp, client)

	// Read from KV store
	secret, err := client.Logical().Read("secret/data/" + v.Path)
	if err != nil {
		return nil, errors.Wrapf(err, "while reading key from kv store at %v", v.Path)
	}

	// Parse key from response
	m, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		return nil, errors.Wrapf(err, "kv store read response from vault is bad")
	}
	kVal, ok := m[v.Field]
	if !ok {
		return nil, errors.Errorf("secret key not found at %v", v.Field)
	}
	kbyte := []byte(kVal.(string))
	klen := len(kbyte)

	// Validate key length suitable for AES
	if klen != 16 && klen != 32 && klen != 64 {
		return nil, errors.Errorf("bad key length %v from vault", klen)
	}
	return kbyte, nil
}
