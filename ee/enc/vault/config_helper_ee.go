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
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type Config struct {
	Addr     string
	RoleID   string
	SecretID string
	Path     string
	Field    string
}

// RegisterVaultFlags registers the required flags to integrate with Vault.
func RegisterFlags(flag *pflag.FlagSet) {
	// The following are Vault options. Applicable for alpha, live, bulk, debug, restore sub-cmds
	flag.String("vault_addr", "localhost:8200",
		"Vault server's ip:port.")
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
	v := Config{
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
