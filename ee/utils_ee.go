// +build !oss

/*
 * Copyright 2021 Dgraph Labs, Inc. All rights reserved.
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package ee

import (
	"io/ioutil"

	"github.com/dgraph-io/dgraph/ee/vault"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/golang/glog"
	"github.com/spf13/viper"
)

// GetKeys returns the ACL and encryption keys as configured by the user
// through the --acl, --encryption_key_file, and --vault flags. On OSS builds,
// this function exits with an error.
func GetKeys(config *viper.Viper) (x.SensitiveByteSlice, x.SensitiveByteSlice) {
	aclSuperFlag := z.NewSuperFlag(config.GetString("acl"))
	aclKey, encKey := vault.GetKeys(config)
	var err error

	aclKeyFile := aclSuperFlag.GetPath("secret-file")
	if aclKeyFile != "" {
		if aclKey != nil {
			glog.Exit("flags: ACL secret key set in both vault and acl flags")
		}
		if aclKey, err = ioutil.ReadFile(aclKeyFile); err != nil {
			glog.Exitf("error reading ACL secret key from file: %s: %s", aclKeyFile, err)
		}
	}
	if l := len(aclKey); aclKey != nil && l < 32 {
		glog.Exitf("ACL secret key must have length of at least 32 bytes, got %d bytes instead", l)
	}

	encKeyFile := config.GetString("encryption_key_file")
	if encKeyFile != "" {
		if encKey != nil {
			glog.Exit("flags: Encryption key set in both vault and encryption_key_file")
		}
		if encKey, err = ioutil.ReadFile(encKeyFile); err != nil {
			glog.Exitf("error reading encryption key from file: %s: %s", encKeyFile, err)
		}
	}
	if l := len(encKey); encKey != nil && l != 16 && l != 32 && l != 64 {
		glog.Exitf("encryption key must have length of 16, 32, or 64 bytes, got %d bytes instead", l)
	}

	return aclKey, encKey
}
