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
	"crypto/aes"
	"crypto/cipher"
	"github.com/dgraph-io/badger/v2/y"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
)

// EeBuild indicates if this is a Enterprise build.
var EeBuild = true

// ReadEncryptionKeyFile returns the encryption key in the given file.
func ReadEncryptionKeyFile(filepath string) []byte {
	if filepath == "" {
		return nil
	}
	k, err := ioutil.ReadFile(filepath)
	x.Checkf(err, "Error reading encryption key file (%v)", filepath)

	// len must be 16,24,32 bytes if given. All other lengths are invalid.
	klen := len(k)
	x.AssertTruef(klen == 16 || klen == 24 || klen == 32,
		"Invalid encryption key length = %v", klen)

	return k
}

// GetWriter wraps a crypto StreamWriter on input Writer given a key file
func GetWriter(filepath string, w io.Writer) (io.Writer, error) {
	// No encryption, return the input writer as is.
	if filepath == "" {
		return w, nil
	}
	// Encryption, wrap crypto StreamWriter on the input Writer.
	c, err := aes.NewCipher(ReadEncryptionKeyFile(filepath))
	if err != nil {
		return nil, err
	}
	iv, err := y.GenerateIV()
	if err != nil {
		return nil, err
	}
	if iv != nil {
		if _, err = w.Write(iv); err != nil {
			return nil, err
		}
	}
	return cipher.StreamWriter{S: cipher.NewCTR(c, iv), W: w}, nil
}

// GetReader returns a crypto StreamReader on the input Reader given a key file.
func GetReader(filepath string, r io.Reader) (io.Reader, error) {
	// No encryption, return input reader as is.
	if filepath == "" {
		return r, nil
	}

	// Encryption, wrap crypto StreamReader on input Reader.
	c, err := aes.NewCipher(ReadEncryptionKeyFile(filepath))
	if err != nil {
		return nil, err
	}
	var iv []byte = make([]byte, 16)
	cnt, err := r.Read(iv)
	if cnt != 16 || err != nil {
		err = errors.Errorf("unable to get IV from encrypted backup. Read %v bytes, err %v ",
			cnt, err)
		return nil, err
	}
	return cipher.StreamReader{S: cipher.NewCTR(c, iv), R: r}, nil
}
