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
	"os"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func ResetConfig(config *viper.Viper) {
	config.Set("encryption_key_file", "")

	config.Set("vault_addr", "http://localhost:8200")
	config.Set("vault_roleID_file", "")
	config.Set("vault_secretID_file", "")
	config.Set("vault_path", "dgraph")
	config.Set("vault_field", "enc_key")
}

func TestNewKeyReader(t *testing.T) {
	config := viper.New()
	flags := &pflag.FlagSet{}
	RegisterFlags(flags)
	config.BindPFlags(flags)

	// Vault and Local combination tests
	// Both Local and Vault options is invalid.
	ResetConfig(config)
	config.Set("encryption_key_file", "blah")
	config.Set("vault_roleID_file", "aaa")
	config.Set("vault_secretID_file", "bbb")
	kR, err := NewKeyReader(config)
	require.Error(t, err)
	require.Nil(t, kR)
	t.Logf("%v", err)

	// RoleID only is invalid.
	ResetConfig(config)
	config.Set("vault_roleID_file", "aaa")
	kR, err = NewKeyReader(config)
	require.Error(t, err)
	require.Nil(t, kR)
	t.Logf("%v", err)

	// SecretID only is invalid.
	ResetConfig(config)
	config.Set("vault_secretID_file", "bbb")
	kR, err = NewKeyReader(config)
	require.Error(t, err)
	require.Nil(t, kR)
	t.Logf("%v", err)

	// RoleID and SecretID given but RoleID file doesn't exist.
	ResetConfig(config)
	config.Set("vault_roleID_file", "aaa")
	config.Set("vault_secretID_file", "bbb")
	kR, err = NewKeyReader(config)
	require.NoError(t, err)
	require.NotNil(t, kR)
	require.IsType(t, &vaultKeyReader{}, kR)
	k, err := kR.ReadKey()
	require.Nil(t, k)
	require.Error(t, err)
	t.Logf("%v", err)

	// RoleID and SecretID given but RoleID file exists. SecretID file doesn't exists.
	ResetConfig(config)
	config.Set("vault_roleID_file", "./dummy_role_id_file")
	config.Set("vault_secretID_file", "bbb")
	kR, err = NewKeyReader(config)
	require.NoError(t, err)
	require.NotNil(t, kR)
	require.IsType(t, &vaultKeyReader{}, kR)
	k, err = kR.ReadKey()
	require.Nil(t, k)
	require.Error(t, err)
	t.Logf("%v", err)

	// RoleID and SecretID given but RoleID file and SecretID file exists.
	ResetConfig(config)
	config.Set("vault_roleID_file", "./dummy_role_id_file")
	config.Set("vault_secretID_file", "./dummy_secret_id_file")
	kR, err = NewKeyReader(config)
	require.NoError(t, err)
	require.NotNil(t, kR)
	require.IsType(t, &vaultKeyReader{}, kR)
	k, err = kR.ReadKey()
	require.Nil(t, k) // still fails because we need to mock Vault server.
	require.Error(t, err)
	t.Logf("%v", err)

	// Bad Encryption Key File
	ResetConfig(config)
	config.Set("encryption_key_file", "blah")
	kR, err = NewKeyReader(config)
	require.NoError(t, err)
	require.NotNil(t, kR)
	require.IsType(t, &localKeyReader{}, kR)
	k, err = kR.ReadKey()
	require.Nil(t, k)
	require.Error(t, err)

	// Nil Encryption Key File
	ResetConfig(config)
	config.Set("encryption_key_file", "")
	kR, err = NewKeyReader(config)
	require.NoError(t, err)
	require.Nil(t, kR)
	t.Logf("%v", err)

	// Bad Length Encryption Key File.
	ResetConfig(config)
	config.Set("encryption_key_file", "./bad-length-enc-key")
	kR, err = NewKeyReader(config)
	require.NoError(t, err)
	require.NotNil(t, kR)
	require.IsType(t, &localKeyReader{}, kR)
	k, err = kR.ReadKey()
	require.Nil(t, k)
	require.Error(t, err)
	t.Logf("%v", err)

	// Good Encryption Key File.
	ResetConfig(config)
	config.Set("encryption_key_file", "./enc-key")
	kR, err = NewKeyReader(config)
	require.NoError(t, err)
	require.NotNil(t, kR)
	require.IsType(t, &localKeyReader{}, kR)
	k, err = kR.ReadKey()
	require.NotNil(t, k)
	require.NoError(t, err)
	t.Logf("%v", err)
}

func keyReaderRun() {

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
	neww, err = GetWriter(ReadEncryptionKeyFile("./enc-key"), f)
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
	newr, err = GetReader(ReadEncryptionKeyFile("./enc-key"), f)
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
