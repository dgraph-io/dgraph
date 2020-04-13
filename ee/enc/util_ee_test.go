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

	"github.com/stretchr/testify/require"
)

func TestGetReaderWriter(t *testing.T) {
	// Test GetWriter()
	f, err := os.Create("/tmp/enc_test")
	require.NoError(t, err)
	defer os.Remove("/tmp/enc_test")

	// empty key file
	neww, err := GetWriter("", f)
	require.NoError(t, err)
	require.Equal(t, f, neww)

	// valid key
	neww, err = GetWriter("./enc-key", f)
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

	// empty key file.
	newr, err := GetReader("", f)
	require.NoError(t, err)
	require.Equal(t, f, newr)

	// valid key
	newr, err = GetReader("./enc-key", f)
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
