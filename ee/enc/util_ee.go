//go:build !oss
// +build !oss

/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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
	"io"

	"github.com/pkg/errors"

	"github.com/dgraph-io/badger/v3/y"
	"github.com/dgraph-io/dgraph/x"
)

// EeBuild indicates if this is a Enterprise build.
var EeBuild = true

// GetWriter wraps a crypto StreamWriter using the input key on the input Writer.
func GetWriter(key x.Sensitive, w io.Writer) (io.Writer, error) {
	// No encryption, return the input writer as is.
	if key == nil {
		return w, nil
	}
	// Encryption, wrap crypto StreamWriter on the input Writer.
	c, err := aes.NewCipher(key)
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

// GetReader wraps a crypto StreamReader using the input key on the input Reader.
func GetReader(key x.Sensitive, r io.Reader) (io.Reader, error) {
	// No encryption, return input reader as is.
	if key == nil {
		return r, nil
	}

	// Encryption, wrap crypto StreamReader on input Reader.
	c, err := aes.NewCipher(key)
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
