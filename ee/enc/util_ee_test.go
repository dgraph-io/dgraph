// +build !oss

/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package enc

import (
	//"io"
	"crypto/cipher"
	"fmt"

	//"net"
	"os"
	"testing"

	//"github.com/hashicorp/vault/api"
	// "github.com/hashicorp/vault/http"
	// "github.com/hashicorp/vault/vault"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func getEncConfig() *viper.Viper {
	config := viper.New()
	flags := &pflag.FlagSet{}
	RegisterFlags(flags)
	config.BindPFlags(flags)
	return config
}

func resetConfig(config *viper.Viper, roleIdFile, secretIdFile, format string) {
	config.Set(encKeyFile, "")

	formatStr := ""
	if format == "" {
		formatStr = "base64"
	}
	config.Set("vault", fmt.Sprintf("addr=%s; role-id-file=%s; secret-id-file=%s; path=%s; "+
		"field=%s; format=%s;",
		// addr
		"http://localhost:8200",
		// role-id-file
		roleIdFile,
		// secret-id-file
		secretIdFile,
		// path
		"secret/data/dgraph",
		// field
		"enc_key",
		// format
		formatStr,
	))
}

// TODO: The function below allows instantiating a real Vault server. But results in go.mod issues.
// func startVaultServer(t *testing.T, kvPath, kvField, kvEncKey string) (net.Listener, *api.Client) {
// 	core, _, rootToken := vault.TestCoreUnsealed(t)
// 	ln, addr := http.TestServer(t, core)
// 	t.Logf("addr = %v", addr)

// 	conf := api.DefaultConfig()
// 	conf.Address = addr
// 	client, err := api.NewClient(conf)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	client.SetToken(rootToken)

// 	err = client.Sys().EnableAuthWithOptions("approle/", &api.EnableAuthOptions{
// 		Type: "approle",
// 	})

// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	t.Logf("approle enabled")

// 	return ln, client
// }

func TestNewKeyReader(t *testing.T) {
	config := getEncConfig()
	// Vault and Local combination tests
	// Both Local and Vault options is invalid.
	resetConfig(config, "aaa", "bbb", "")
	config.Set(encKeyFile, "blah")
	kr, err := newKeyReader(config)
	require.Error(t, err)
	require.Nil(t, kr)

	// RoleID only is invalid.
	resetConfig(config, "aaa", "", "")
	kr, err = newKeyReader(config)
	require.Error(t, err)
	require.Nil(t, kr)

	// SecretID only is invalid.
	resetConfig(config, "", "bbb", "")
	kr, err = newKeyReader(config)
	require.Error(t, err)
	require.Nil(t, kr)

	// RoleID and SecretID given but RoleID file doesn't exist.
	resetConfig(config, "aaa", "bbb", "")
	k, err := ReadKey(config)
	require.Nil(t, k)
	require.Error(t, err)

	// RoleID and SecretID given but RoleID file exists. SecretID file doesn't exists.
	resetConfig(config, "./test-fixtures/dummy_role_id_file", "bbb", "")
	kr, err = newKeyReader(config)
	require.NoError(t, err)
	require.NotNil(t, kr)
	require.IsType(t, &vaultKeyReader{}, kr)
	k, err = kr.readKey()
	require.Nil(t, k)
	require.Error(t, err)

	// Bad vault_format. Must be raw or base64.
	resetConfig(config,
		"./test-fixtures/dummy_role_id_file",
		"./test-fixtures/dummy_secret_id_file",
		// format
		"foo") // error.
	kr, err = newKeyReader(config)
	require.Error(t, err)
	require.Nil(t, kr)

	// RoleID and SecretID given but RoleID file and SecretID file exists and is valid.
	resetConfig(config,
		"./test-fixtures/dummy_role_id_file",
		"./test-fixtures/dummy_secret_id_file",
		"")
	//nl, _ := startVaultServer(t, "dgraph", "enc_key", "1234567890123456")

	kr, err = newKeyReader(config)
	require.NoError(t, err)
	require.NotNil(t, kr)
	require.IsType(t, &vaultKeyReader{}, kr)
	k, err = kr.readKey()
	require.Nil(t, k) // still fails because we need to mock Vault server.
	require.Error(t, err)
	//nl.Close()

	// Bad Encryption Key File
	resetConfig(config, "", "", "")
	config.Set(encKeyFile, "blah")
	kr, err = newKeyReader(config)
	require.NoError(t, err)
	require.NotNil(t, kr)
	require.IsType(t, &localKeyReader{}, kr)
	k, err = kr.readKey()
	require.Nil(t, k)
	require.Error(t, err)

	// Nil Encryption Key File
	resetConfig(config, "", "", "")
	config.Set(encKeyFile, "")
	kr, err = newKeyReader(config)
	require.NoError(t, err)
	require.Nil(t, kr)

	// Bad Length Encryption Key File.
	resetConfig(config, "", "", "")
	config.Set(encKeyFile, "./test-fixtures/bad-length-enc-key")
	kr, err = newKeyReader(config)
	require.NoError(t, err)
	require.NotNil(t, kr)
	require.IsType(t, &localKeyReader{}, kr)
	k, err = kr.readKey()
	require.Nil(t, k)
	require.Error(t, err)

	// Good Encryption Key File.
	resetConfig(config, "", "", "")
	config.Set(encKeyFile, "./test-fixtures/enc-key")
	kr, err = newKeyReader(config)
	require.NoError(t, err)
	require.NotNil(t, kr)
	require.IsType(t, &localKeyReader{}, kr)
	k, err = kr.readKey()
	require.NotNil(t, k)
	require.NoError(t, err)
}

func TestGetReaderWriter(t *testing.T) {
	// Test GetWriter()
	f, err := os.Create("/tmp/enc_test")
	require.NoError(t, err)
	defer os.Remove("/tmp/enc_test")

	// empty key
	neww, err := GetWriter(nil, f)
	require.NoError(t, err)
	require.Equal(t, f, neww)

	// valid key
	config := getEncConfig()
	config.Set(encKeyFile, "./test-fixtures/enc-key")
	kr, err := newKeyReader(config)
	require.NoError(t, err)
	require.NotNil(t, kr)
	require.IsType(t, &localKeyReader{}, kr)
	k, err := kr.readKey()
	require.NotNil(t, k)
	require.NoError(t, err)
	neww, err = GetWriter(k, f)
	require.NoError(t, err)
	require.NotEqual(t, f, neww)
	require.IsType(t, cipher.StreamWriter{}, neww)
	require.Equal(t, neww.(cipher.StreamWriter).W, f)
	// lets encrypt
	data := []byte("this is plaintext form")
	_, err = neww.Write(data)
	require.NoError(t, err)
	f.Close()

	// Test GetReader()
	f, err = os.Open("/tmp/enc_test")
	require.NoError(t, err)

	// empty key.
	newr, err := GetReader(nil, f)
	require.NoError(t, err)
	require.Equal(t, f, newr)

	// valid key
	newr, err = GetReader(k, f)
	require.NoError(t, err)
	require.NotEqual(t, f, newr)
	require.IsType(t, cipher.StreamReader{}, newr)
	require.Equal(t, newr.(cipher.StreamReader).R, f)

	// lets decrypt
	plain := make([]byte, len(data))
	_, err = newr.Read(plain)
	require.NoError(t, err)
	require.Equal(t, data, plain)
	f.Close()
}
