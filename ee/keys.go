// +build oss

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
<<<<<<< HEAD:ee/utils.go
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
=======
	"fmt"

>>>>>>> master:ee/keys.go
	"github.com/spf13/viper"
)

// GetKeys returns the ACL and encryption keys as configured by the user
// through the --acl, --encryption, and --vault flags. On OSS builds,
<<<<<<< HEAD:ee/utils.go
// this function exits with an error.
func GetKeys(config *viper.Viper) (x.SensitiveByteSlice, x.SensitiveByteSlice) {
	glog.Exit("flags: acl / encryption is an enterprise-only feature")
	return nil, nil
=======
// this function always returns an error.
func GetKeys(config *viper.Viper) (*Keys, error) {
	return nil, fmt.Errorf(
		"flags: acl / encryption is an enterprise-only feature")
>>>>>>> master:ee/keys.go
}
