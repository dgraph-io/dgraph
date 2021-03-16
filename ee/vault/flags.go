/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
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
	"github.com/dgraph-io/ristretto/z"
	"github.com/spf13/pflag"
)

const (
	flagVault        = "vault"
	flagAddr         = "addr"
	flagRoleIdFile   = "role-id-file"
	flagSecretIdFile = "secret-id-file"
	flagPath         = "path"
	flagAclField     = "acl-field"
	flagAclFormat    = "acl-format"
	flagEncField     = "enc-field"
	flagEncFormat    = "enc-format"

	defaultConfig = "addr=http://localhost:8200; " +
		"path=secret/data/dgraph; " +
		"acl-format=base64; " +
		"enc-format=base64;"
)

var helpText = z.NewSuperFlagHelp(defaultConfig).
	Head("Vault options").
	Flag(flagAddr, "Vault server address (format: http://ip:port).").
	Flag(flagRoleIdFile, "Vault RoleID file, used for AppRole authentication.").
	Flag(flagSecretIdFile, "Vault SecretID file, used for AppRole authentication.").
	Flag(flagPath,
		"Vault KV store path (e.g. 'secret/data/dgraph' for KV V2, 'kv/dgraph' for KV V1).").
	Flag(flagAclField, "Vault field containing ACL key.").
	Flag(flagAclFormat, "ACL key format, can be 'raw' or 'base64'.").
	Flag(flagEncField, "Vault field containing encryption key.").
	Flag(flagEncFormat, "Encryption key format, can be 'raw' or 'base64'.").
	String()

func RegisterFlags(flag *pflag.FlagSet) {
	flag.String(flagVault, defaultConfig, helpText)
}
