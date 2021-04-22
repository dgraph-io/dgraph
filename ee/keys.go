//go:build oss
// +build oss

/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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
	"fmt"

	"github.com/spf13/viper"
)

// GetKeys returns the ACL and encryption keys as configured by the user
// through the --acl, --encryption, and --vault flags. On OSS builds,
// this function always returns an error.
func GetKeys(config *viper.Viper) (*Keys, error) {
	return nil, fmt.Errorf(
		"flags: acl / encryption is an enterprise-only feature")
}
