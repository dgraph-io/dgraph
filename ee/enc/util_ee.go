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
)

const badgerEncFlag string = "badger.encryption_key"

// BadgerEncryptionKeyFlag exposes the badger.encryption_key flag to sub-cmds.
func BadgerEncryptionKeyFlag(flag *pflag.FlagSet) {
	flag.String(badgerEncFlag, "",
		"Specifies badger encryption key. Must be 16/24/32 bytes that determines "+
			"AES-128/192/256 encryption algorithm respectively.")
}

// GetEncryptionKeyString returns the configured key
func GetEncryptionKeyString(c *viper.Viper) string {
	k := c.GetString(badgerEncFlag)
	// zero out the key from memory now.
	c.Set(badgerEncFlag, "")

	// len must be 16,24,32 bytes if given. 0 otherwise. All other lengths are invalid.
	klen := len(k)
	x.AssertTruef(klen == 0 || klen == 16 || klen == 24 || klen == 32,
		"Invalid Badger encryption key length = %v", klen)
	return k
}
