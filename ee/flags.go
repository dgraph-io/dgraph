/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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

package ee

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/pflag"

	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
)

// Keys holds the configuration for ACL and encryption.
type Keys struct {
	AclKey        x.Sensitive
	AclAccessTtl  time.Duration
	AclRefreshTtl time.Duration
	EncKey        x.Sensitive
}

const (
	flagAcl           = "acl"
	flagAclAccessTtl  = "access-ttl"
	flagAclRefreshTtl = "refresh-ttl"
	flagAclSecretFile = "secret-file"

	flagEnc        = "encryption"
	flagEncKeyFile = "key-file"

	flagVault             = "vault"
	flagVaultAddr         = "addr"
	flagVaultRoleIdFile   = "role-id-file"
	flagVaultSecretIdFile = "secret-id-file"
	flagVaultPath         = "path"
	flagVaultAclField     = "acl-field"
	flagVaultAclFormat    = "acl-format"
	flagVaultEncField     = "enc-field"
	flagVaultEncFormat    = "enc-format"
)

func RegisterAclAndEncFlags(flag *pflag.FlagSet) {
	registerAclFlag(flag)
	registerEncFlag(flag)
	registerVaultFlag(flag, true, true)
}

func RegisterEncFlag(flag *pflag.FlagSet) {
	registerEncFlag(flag)
	registerVaultFlag(flag, false, true)
}

var (
	AclDefaults = fmt.Sprintf("%s=%s; %s=%s; %s=%s",
		flagAclAccessTtl, "6h",
		flagAclRefreshTtl, "30d",
		flagAclSecretFile, "")
	EncDefaults = fmt.Sprintf("%s=%s", flagEncKeyFile, "")
)

func vaultDefaults(aclEnabled, encEnabled bool) string {
	var configBuilder strings.Builder
	fmt.Fprintf(&configBuilder, "%s=%s; %s=%s; %s=%s; %s=%s",
		flagVaultAddr, "http://localhost:8200",
		flagVaultRoleIdFile, "",
		flagVaultSecretIdFile, "",
		flagVaultPath, "secret/data/dgraph")
	if aclEnabled {
		fmt.Fprintf(&configBuilder, "; %s=%s; %s=%s",
			flagVaultAclField, "",
			flagVaultAclFormat, "base64")
	}
	if encEnabled {
		fmt.Fprintf(&configBuilder, "; %s=%s; %s=%s",
			flagVaultEncField, "",
			flagVaultEncFormat, "base64")
	}
	return configBuilder.String()
}

func registerVaultFlag(flag *pflag.FlagSet, aclEnabled, encEnabled bool) {
	// Generate default configuration.
	config := vaultDefaults(aclEnabled, encEnabled)

	// Generate help text.
	helpBuilder := z.NewSuperFlagHelp(config).
		Head("Vault options").
		Flag(flagVaultAddr, "Vault server address (format: http://ip:port).").
		Flag(flagVaultRoleIdFile, "Vault RoleID file, used for AppRole authentication.").
		Flag(flagVaultSecretIdFile, "Vault SecretID file, used for AppRole authentication.").
		Flag(flagVaultPath, "Vault KV store path (e.g. 'secret/data/dgraph' for KV V2, "+
			"'kv/dgraph' for KV V1).")
	if aclEnabled {
		helpBuilder = helpBuilder.
			Flag(flagVaultAclField, "Vault field containing ACL key.").
			Flag(flagVaultAclFormat, "ACL key format, can be 'raw' or 'base64'.")
	}
	if encEnabled {
		helpBuilder = helpBuilder.
			Flag(flagVaultEncField, "Vault field containing encryption key.").
			Flag(flagVaultEncFormat, "Encryption key format, can be 'raw' or 'base64'.")
	}
	helpText := helpBuilder.String()

	// Register flag.
	flag.String(flagVault, config, helpText)
}

func registerAclFlag(flag *pflag.FlagSet) {
	helpText := z.NewSuperFlagHelp(AclDefaults).
		Head("[Enterprise Feature] ACL options").
		Flag("secret-file",
			"The file that stores the HMAC secret, which is used for signing the JWT and "+
				"should have at least 32 ASCII characters. Required to enable ACLs.").
		Flag("access-ttl",
			"The TTL for the access JWT.").
		Flag("refresh-ttl",
			"The TTL for the refresh JWT.").
		String()
	flag.String(flagAcl, AclDefaults, helpText)
}

func registerEncFlag(flag *pflag.FlagSet) {
	helpText := z.NewSuperFlagHelp(EncDefaults).
		Head("[Enterprise Feature] Encryption At Rest options").
		Flag("key-file", "The file that stores the symmetric key of length 16, 24, or 32 bytes."+
			"The key size determines the chosen AES cipher (AES-128, AES-192, and AES-256 respectively).").
		String()
	flag.String(flagEnc, EncDefaults, helpText)
}

func BuildEncFlag(filename string) string {
	return fmt.Sprintf("key-file=%s;", filename)
}
