// +build !oss

/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package ee

import (
	"io/ioutil"

	"github.com/dgraph-io/dgraph/ee/vault"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/spf13/viper"
)

func GetKeys(config *viper.Viper) (aclKey, encKey x.SensitiveByteSlice) {
	const (
		flagAclKeyFile = "acl_secret_file"
		flagEncKeyFile = "encryption_key_file"
	)

	aclKey, encKey = vault.GetKeys(config)
	var err error

	aclKeyFile := config.GetString(flagAclKeyFile)
	if aclKeyFile != "" {
		if aclKey != nil {
			glog.Exit("flags: ACL secret key set in both vault and acl_secret_file")
		}
		aclKey, err = ioutil.ReadFile(aclKeyFile)
		if err != nil {
			glog.Exitf("error reading ACL secret key from file: %s: %s", aclKeyFile, err)
		}
	}
	if aclKey != nil {
		if l := len(aclKey); l < 32 {
			glog.Exitf("ACL secret key must have length of at least 32 bytes, got %d bytes instead", l)
		}
	}

	encKeyFile := config.GetString(flagEncKeyFile)
	if encKeyFile != "" {
		if encKey != nil {
			glog.Exit("flags: Encryption key set in both vault and encryption_key_file")
		}
		encKey, err = ioutil.ReadFile(encKeyFile)
		if err != nil {
			glog.Exitf("error reading encryption key from file: %s: %s", encKeyFile, err)
		}
	}
	if encKey != nil {
		if l := len(encKey); l != 16 && l != 32 && l != 64 {
			glog.Exitf("encryption key must have length of 16, 32, or 64 bytes, got %d bytes instead", l)
		}
	}

	return
}
