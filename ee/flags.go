/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package ee

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/spf13/pflag"

	"github.com/dgraph-io/ristretto/v2/z"
	"github.com/hypermodeinc/dgraph/v24/x"
)

// Keys holds the configuration for ACL and encryption.
type Keys struct {
	AclJwtAlg         jwt.SigningMethod
	AclSecretKey      interface{}
	AclPublicKey      interface{}
	AclSecretKeyBytes x.Sensitive // we need this to compute hash for auth
	AclAccessTtl      time.Duration
	AclRefreshTtl     time.Duration
	EncKey            x.Sensitive
}

const (
	flagAcl           = "acl"
	flagAclAccessTtl  = "access-ttl"
	flagAclRefreshTtl = "refresh-ttl"
	flagAclJwtAlg     = "jwt-alg"
	flagAclKeyFile    = "secret-file"

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
	AclDefaults = fmt.Sprintf("%s=%s; %s=%s; %s=%s; %s=%s",
		flagAclAccessTtl, "6h",
		flagAclRefreshTtl, "30d",
		flagAclJwtAlg, "HS256",
		flagAclKeyFile, "")
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
		Flag("jwt-alg", "JWT signing algorithm, default HS256").
		Flag("secret-file",
			"The file that stores secret key or private key, which is used to sign the ACL JWT.").
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
