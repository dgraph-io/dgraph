//go:build !oss
// +build !oss

/*
 * Copyright 2022 Dgraph Labs, Inc. All rights reserved.
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package ee

import (
	"fmt"
	"io/ioutil"

	"github.com/spf13/viper"

	"github.com/dgraph-io/ristretto/z"
)

// GetKeys returns the ACL and encryption keys as configured by the user
// through the --acl, --encryption, and --vault flags. On OSS builds,
// this function always returns an error.
func GetKeys(config *viper.Viper) (*Keys, error) {
	keys := &Keys{}

	aclSuperFlag := z.NewSuperFlag(config.GetString("acl")).MergeAndCheckDefault(AclDefaults)
	encSuperFlag := z.NewSuperFlag(config.GetString("encryption")).MergeAndCheckDefault(EncDefaults)

	// Get AclKey and EncKey from vault / acl / encryption SuperFlags
	keys.AclKey, keys.EncKey = vaultGetKeys(config)
	aclKeyFile := aclSuperFlag.GetPath(flagAclSecretFile)
	if aclKeyFile != "" {
		if keys.AclKey != nil {
			return nil, fmt.Errorf("flags: ACL secret key set in both vault and acl flags")
		}
		var err error
		if keys.AclKey, err = ioutil.ReadFile(aclKeyFile); err != nil {
			return nil, fmt.Errorf("error reading ACL secret key from file: %s: %s", aclKeyFile, err)
		}
	}
	if l := len(keys.AclKey); keys.AclKey != nil && l < 32 {
		return nil, fmt.Errorf(
			"ACL secret key must have length of at least 32 bytes, got %d bytes instead", l)
	}
	encKeyFile := encSuperFlag.GetPath(flagEncKeyFile)
	if encKeyFile != "" {
		if keys.EncKey != nil {
			return nil, fmt.Errorf("flags: Encryption key set in both vault and encryption flags")
		}
		var err error
		if keys.EncKey, err = ioutil.ReadFile(encKeyFile); err != nil {
			return nil, fmt.Errorf("error reading encryption key from file: %s: %s", encKeyFile, err)
		}
	}
	if l := len(keys.EncKey); keys.EncKey != nil && l != 16 && l != 32 && l != 64 {
		return nil, fmt.Errorf(
			"encryption key must have length of 16, 32, or 64 bytes, got %d bytes instead", l)
	}

	// Get remaining keys
	keys.AclAccessTtl = aclSuperFlag.GetDuration(flagAclAccessTtl)
	keys.AclRefreshTtl = aclSuperFlag.GetDuration(flagAclRefreshTtl)

	return keys, nil
}
