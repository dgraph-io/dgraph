// +build !oss

/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package vault

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/builtin/credential/approle"
	"github.com/hashicorp/vault/http"
	"github.com/hashicorp/vault/vault"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

const (
	path = "secret/data/dgraph"
)

func TestGetKeys(t *testing.T) {
	const (
		aclField = "acl-field"
		encField = "enc-field"

		aclKey = "01234567890123456789012345678901"
		encKey = "0123456789012345"
	)

	// Set up Vault server.
	client := newTestServer(t)
	roleIdFile, secretIdFile := addTestRole(t, client)

	// Add ACL and encryption keys to Vault.
	_, err := client.Logical().Write(path, map[string]interface{}{
		aclField: base64.StdEncoding.EncodeToString([]byte(aclKey)),
		encField: base64.StdEncoding.EncodeToString([]byte(encKey)),
	})
	require.NoError(t, err)

	// Configure Vault in Dgraph.
	config := viper.New()
	config.Set(flagVault, fmt.Sprintf(
		`%s=%s;%s=%s;%s=%s;%s=%s;%s=%s;%s=%s;%s=%s;%s=%s;`,
		flagAddr, client.Address(),
		flagRoleIdFile, roleIdFile,
		flagSecretIdFile, secretIdFile,
		flagPath, path,
		flagAclField, aclField,
		flagAclFormat, "base64",
		flagEncField, encField,
		flagEncFormat, "base64"))

	// Retrieve keys using Dgraph and compare.
	aclKeyGot, encKeyGot := GetKeys(config)
	require.Equal(t, aclKey, string(aclKeyGot))
	require.Equal(t, encKey, string(encKeyGot))
}

// newServer starts an AppRole-enabled Vault server and returns a client for it.
func newTestServer(t *testing.T) *api.Client {
	// Start up Vault server.
	vault.AddTestCredentialBackend("approle", approle.Factory)
	core, _, rootToken := vault.TestCoreUnsealed(t)
	_, address := http.TestServer(t, core)

	// Create configuration.
	config := api.DefaultConfig()
	config.Address = address

	// Create Vault client.
	client, err := api.NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	client.SetToken(rootToken)
	client.Sys().EnableAuthWithOptions("approle/", &api.EnableAuthOptions{Type: "approle"})

	return client
}

// addTestRole adds a new role to Vault and returns its credentials.
func addTestRole(t *testing.T, client *api.Client) (roleIdFile, secretIdFile string) {
	const roleName = "test-role"
	const policyName = "test-policy"

	// Add roleName to Vault.
	_, err := client.Logical().Write(fmt.Sprintf("auth/approle/role/%s", roleName), map[string]interface{}{
		"secret_id_ttl":      "10m",
		"token_num_uses":     10,
		"token_ttl":          "20m",
		"token_max_ttl":      "30m",
		"secret_id_num_uses": 40,
	})
	require.NoError(t, err)

	// Get the role ID of roleName.
	secret, err := client.Logical().Read(fmt.Sprintf("auth/approle/role/%s/role-id", roleName))
	require.NoError(t, err)
	roleId := secret.Data["role_id"].(string)
	roleIdFile = writeTempFile(t, roleId)

	// Generate a secret ID for roleName.
	secret, err = client.Logical().Write(fmt.Sprintf("auth/approle/role/%s/secret-id", roleName), nil)
	require.NoError(t, err)
	secretId := secret.Data["secret_id"].(string)
	secretIdFile = writeTempFile(t, secretId)

	// Create a new policy for this role to read from `path`.
	_, err = client.Logical().Write(fmt.Sprintf("sys/policies/acl/%s", policyName), map[string]interface{}{
		"policy": fmt.Sprintf(`
		path "%s"
		{
			capabilities = ["read"]
		}`, path),
	})
	require.NoError(t, err)
	secret, err = client.Logical().Write(fmt.Sprintf("auth/approle/role/%s/policies", roleName), map[string]interface{}{
		"token_policies": []string{policyName},
	})
	require.NoError(t, err)

	return
}

// writeTempFile creates a tempfile with the required data and returns the filename.
func writeTempFile(t *testing.T, data string) string {
	file, err := ioutil.TempFile("", "")
	require.NoError(t, err)
	defer file.Close()
	t.Cleanup(func() { os.Remove(file.Name()) })

	_, err = file.WriteString(data)
	require.NoError(t, err)

	return file.Name()
}
