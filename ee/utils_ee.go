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
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

// GetEEFeaturesList returns a list of Enterprise Features that are available.
func GetEEFeaturesList() []string {
	if !worker.EnterpriseEnabled() {
		return nil
	}
	var ee []string
	if len(worker.Config.HmacSecret) > 0 {
		ee = append(ee, "acl")
	}
	if x.WorkerConfig.EncryptionKey != nil {
		ee = append(ee, "encryption_at_rest", "encrypted_backup_restore", "encrypted_export")
	} else {
		ee = append(ee, "backup_restore")
	}
	if x.WorkerConfig.Audit {
		ee = append(ee, "audit")
	}
	if worker.Config.ChangeDataConf != "" {
		ee = append(ee, "cdc")
	}
	return ee
}
