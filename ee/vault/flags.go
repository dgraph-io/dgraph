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
	"fmt"

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
)

var (
	defaultConfig = fmt.Sprintf("%s=%s; %s=%s; %s=%s; %s=%s",
		flagAddr, "http://localhost:8200",
		flagRoleIdFile, "",
		flagSecretIdFile, "",
		flagPath, "secret/data/dgraph")

	encConfig = fmt.Sprintf("%s=%s; %s=%s",
		flagEncField, "",
		flagEncFormat, "base64")

	aclConfig = fmt.Sprintf("%s=%s; %s=%s",
		flagAclField, "",
		flagAclFormat, "base64")

	helpText = z.NewSuperFlagHelp(encConfig).
			Head("Vault options").
			Flag(flagAddr, "Vault server address (format: http://ip:port).").
			Flag(flagRoleIdFile, "Vault RoleID file, used for AppRole authentication.").
			Flag(flagSecretIdFile, "Vault SecretID file, used for AppRole authentication.").
			Flag(flagPath, "Vault KV store path (e.g. 'secret/data/dgraph' for KV V2, "+
			"'kv/dgraph' for KV V1).").
		Flag(flagEncField, "Vault field containing encryption key.").
		Flag(flagEncFormat, "Encryption key format, can be 'raw' or 'base64'.").
		String()

	aclAndEncText = z.NewSuperFlagHelp(encConfig).
			Head("Vault options").
			Flag(flagAddr, "Vault server address (format: http://ip:port).").
			Flag(flagRoleIdFile, "Vault RoleID file, used for AppRole authentication.").
			Flag(flagSecretIdFile, "Vault SecretID file, used for AppRole authentication.").
			Flag(flagPath, "Vault KV store path (e.g. 'secret/data/dgraph' for KV V2, "+
			"'kv/dgraph' for KV V1).").
		Flag(flagEncField, "Vault field containing encryption key.").
		Flag(flagEncFormat, "Encryption key format, can be 'raw' or 'base64'.").
		Flag(flagAclField, "Vault field containing ACL key.").
		Flag(flagAclFormat, "ACL key format, can be 'raw' or 'base64'.").
		String()
)

const (
	EncryptionDefaults = `key-file=;`
	AclDefaults        = `access-ttl=6h; refresh-ttl=30d; secret-file=;`
)

func RegisterEncFlag(flag *pflag.FlagSet) {
	registerEncFlag(flag)
	flag.String(flagVault, fmt.Sprintf("%s; %s", defaultConfig, encConfig), helpText)
}

func registerEncFlag(flag *pflag.FlagSet) {
	flag.String("encryption", EncryptionDefaults, z.NewSuperFlagHelp(EncryptionDefaults).
		Head("[Enterprise Feature] Encryption At Rest options").
		Flag("key-file",
			"The file that stores the symmetric key of length 16, 24, or 32 bytes. The key size"+
				" determines the chosen AES cipher (AES-128, AES-192, and AES-256 respectively).").
		String())

}

func RegisterAclAndEncFlags(flag *pflag.FlagSet) {
	registerEncFlag(flag)

	flag.String("acl", AclDefaults, z.NewSuperFlagHelp(AclDefaults).
		Head("[Enterprise Feature] ACL options").
		Flag("secret-file",
			"The file that stores the HMAC secret, which is used for signing the JWT and "+
				"should have at least 32 ASCII characters. Required to enable ACLs.").
		Flag("access-ttl",
			"The TTL for the access JWT.").
		Flag("refresh-ttl",
			"The TTL for the refresh JWT.").
		String())

	conf := fmt.Sprintf("%s; %s; %s", defaultConfig, encConfig, aclConfig)
	flag.String(flagVault, conf, aclAndEncText)
}
