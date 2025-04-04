/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"encoding/base64"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/golang/glog"
	"github.com/hashicorp/vault/api"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/dgraph-io/ristretto/v2/z"
)

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

var (
	AclDefaults = fmt.Sprintf("%s=%s; %s=%s; %s=%s; %s=%s",
		flagAclAccessTtl, "6h",
		flagAclRefreshTtl, "30d",
		flagAclJwtAlg, "HS256",
		flagAclKeyFile, "")
	EncDefaults = fmt.Sprintf("%s=%s", flagEncKeyFile, "")
)

func vaultGetKeys(config *viper.Viper) (aclKey, encKey Sensitive) {
	// Avoid querying Vault unless the flag has been explicitly set.
	if !config.IsSet(flagVault) {
		return
	}

	vaultString := config.GetString(flagVault)
	vaultStringDefault := vaultDefaults(true, true)
	vaultFlag := z.NewSuperFlag(vaultString).MergeAndCheckDefault(vaultStringDefault)
	vaultConfig, err := vaultParseFlag(vaultFlag)
	if err != nil {
		glog.Exit(err)
	}

	// Avoid querying Vault unless there is data we want to retrieve from Vault.
	if vaultConfig.aclField == "" && vaultConfig.encField == "" {
		return
	}

	client, err := vaultNewClient(vaultConfig.addr, vaultConfig.roleIdFile, vaultConfig.secretIdFile)
	if err != nil {
		glog.Exit(err)
	}

	kv, err := vaultGetKvStore(client, vaultConfig.path)
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

// vaultKvStore represents a KV store retrieved from the Vault KV Secrets Engine.
type vaultKvStore map[string]interface{}

// vaultGetKvStore fetches a KV store from located at path.
func vaultGetKvStore(client *api.Client, path string) (vaultKvStore, error) {
	secret, err := client.Logical().Read(path)
	if err != nil {
		return nil, fmt.Errorf("vault: error retrieving path %s: %s", path, err)
	}
	if secret == nil || secret.Data == nil {
		return nil, fmt.Errorf("vault: error retrieving path %s: empty response", path)
	}

	var kv vaultKvStore
	kv, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		glog.Infof("vault: failed to parse response in KV V2 format, falling back to V1")
		kv = secret.Data
	}

	return kv, nil
}

// getSensitiveBytes retrieves a value from a kvStore, decoding it if necessary.
func (kv vaultKvStore) getSensitiveBytes(field, format string) (Sensitive, error) {
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
	var valueBytes Sensitive
	var err error
	if format == "base64" {
		valueBytes, err = base64.StdEncoding.DecodeString(valueString)
		if err != nil {
			return nil, fmt.Errorf(
				"vault: key '%s' could not be decoded as a base64 string: %s", field, err)
		}
	} else {
		valueBytes = Sensitive(valueString)
	}

	return valueBytes, nil
}

// vaultNewClient creates an AppRole-authenticated Vault client using the provided credentials.
func vaultNewClient(address, roleIdPath, secretIdPath string) (*api.Client, error) {
	// Connect to Vault.
	client, err := api.NewClient(&api.Config{Address: address})
	if err != nil {
		return nil, fmt.Errorf("vault: error creating client: %s", err)
	}

	// Read Vault credentials from disk.
	loginData := make(map[string]interface{}, 2)
	roleId, err := os.ReadFile(roleIdPath)
	if err != nil {
		return nil, fmt.Errorf("vault: error reading from role ID file: %s", err)
	}
	loginData["role_id"] = string(roleId)
	// If we configure a bound_cidr_list in Vault, we don't need to use a secret_id.
	if secretIdPath != "" {
		secretId, err := os.ReadFile(secretIdPath)
		if err != nil {
			return nil, fmt.Errorf("vault: error reading from secret ID file: %s", err)
		}
		loginData["secret_id"] = string(secretId)
	}

	// Login into Vault with AppRole authentication.
	secret, err := client.Logical().Write("auth/approle/login", loginData)
	if err != nil {
		return nil, fmt.Errorf("vault: login error: %s", err)
	}
	if secret == nil || secret.Auth == nil {
		return nil, fmt.Errorf("vault: login error: empty response")
	}
	client.SetToken(secret.Auth.ClientToken)

	return client, nil
}

type vaultConfig struct {
	addr         string
	roleIdFile   string
	secretIdFile string
	path         string
	aclField     string
	aclFormat    string
	encField     string
	encFormat    string
}

// vaultParseFlag parses and validates a Vault SuperFlag.
func vaultParseFlag(flag *z.SuperFlag) (*vaultConfig, error) {
	// Helper functions to validate flags.
	validateRequired := func(field, value string) error {
		if value == "" {
			return fmt.Errorf("vault: %s field is missing, but is required", field)
		}
		return nil
	}
	validateFormat := func(field, value string) error {
		if value != "base64" && value != "raw" {
			return fmt.Errorf("vault: %s field must be 'base64' or 'raw', found '%s'", field, value)
		}
		return nil
	}

	// Parse and validate flags.
	addr := flag.GetString(flagVaultAddr)
	if err := validateRequired(flagVaultAddr, addr); err != nil {
		return nil, err
	}
	roleIdFile := flag.GetPath(flagVaultRoleIdFile)
	if err := validateRequired(flagVaultRoleIdFile, roleIdFile); err != nil {
		return nil, err
	}
	secretIdFile := flag.GetPath(flagVaultSecretIdFile)
	path := flag.GetString(flagVaultPath)
	if err := validateRequired(flagVaultPath, path); err != nil {
		return nil, err
	}
	aclFormat := flag.GetString(flagVaultAclFormat)
	if err := validateFormat(flagVaultAclFormat, aclFormat); err != nil {
		return nil, err
	}
	encFormat := flag.GetString(flagVaultEncFormat)
	if err := validateFormat(flagVaultEncFormat, encFormat); err != nil {
		return nil, err
	}
	aclField := flag.GetString(flagVaultAclField)
	encField := flag.GetString(flagVaultEncField)
	if aclField == "" && encField == "" {
		return nil, fmt.Errorf(
			"vault: at least one of fields '%s' or '%s' must be provided",
			flagVaultAclField, flagVaultEncField)
	}

	config := &vaultConfig{
		addr:         addr,
		roleIdFile:   roleIdFile,
		secretIdFile: secretIdFile,
		path:         path,
		aclField:     aclField,
		aclFormat:    aclFormat,
		encField:     encField,
		encFormat:    encFormat,
	}
	return config, nil
}

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
