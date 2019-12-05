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
	"github.com/dgraph-io/dgraph/x"
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
	x.Checkf(err, "Error reading Encryption key file (%v)", filepath)

	// len must be 16,24,32 bytes if given. All other lengths are invalid.
	klen := len(k)
	x.AssertTruef(klen == 16 || klen == 24 || klen == 32,
		"Invalid encryption key length = %v", klen)

	return k
}
