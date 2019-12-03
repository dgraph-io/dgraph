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
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"io/ioutil"
)

const encFile string = "encryption_key_file"

// BadgerEncryptionKeyFile exposes the badger.encryption_key_file flag to sub-cmds.
func BadgerEncryptionKeyFile(flag *pflag.FlagSet) {
	flag.String(encFile, "",
		"The file that stores the data encryption key. The key size must be 16, 24, or 32 bytes long. " + 
		"The key size determines the corresponding block size for AES encryption " +
		"(AES-128, AES-192, and AES-256 respectively). Enterprise feature.")
}

// GetEncryptionKeyString returns the configured key
func GetEncryptionKeyString(c *viper.Viper) string {
	f := c.GetString(encFile)
	if f == "" {
		return ""
	}
	k, err := ioutil.ReadFile(f)
	x.Checkf(err, "Error reading Badger Encryption key file (%v)", f)

	// len must be 16,24,32 bytes if given. 0 otherwise. All other lengths are invalid.
	klen := len(k)
	x.AssertTruef(klen == 16 || klen == 24 || klen == 32,
		"Invalid Badger encryption key length = %v", klen)

	return string(k)
}
