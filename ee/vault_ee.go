//go:build !oss
// +build !oss

/*
 * Copyright 2022 Dgraph Labs, Inc. All rights reserved.
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package ee

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"reflect"

	"github.com/golang/glog"
	"github.com/hashicorp/vault/api"
	"github.com/spf13/viper"

	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
)

func vaultGetKeys(config *viper.Viper) (aclKey, encKey x.Sensitive) {
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
func (kv vaultKvStore) getSensitiveBytes(field, format string) (x.Sensitive, error) {
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
	var valueBytes x.Sensitive
	var err error
	if format == "base64" {
		valueBytes, err = base64.StdEncoding.DecodeString(valueString)
		if err != nil {
			return nil, fmt.Errorf(
				"vault: key '%s' could not be decoded as a base64 string: %s", field, err)
		}
	} else {
		valueBytes = x.Sensitive(valueString)
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
	roleId, err := ioutil.ReadFile(roleIdPath)
	if err != nil {
		return nil, fmt.Errorf("vault: error reading from role ID file: %s", err)
	}
	loginData["role_id"] = string(roleId)
	// If we configure a bound_cidr_list in Vault, we don't need to use a secret_id.
	if secretIdPath != "" {
		secretId, err := ioutil.ReadFile(secretIdPath)
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
