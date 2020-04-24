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
)

// List all Enterprise Features here.
const (
	acl              = "acl"
	backupRestore    = "backup_restore"
	encAtRest        = "encryption_at_rest"
	encBackupRestore = "encrypted_backup_restore"
	encExport        = "encrypted_export"
)

// GetEEFeaturesList returns a list of Enterprise Features that are available.
func GetEEFeaturesList() []string {
	if !worker.EnterpriseEnabled() {
		return nil
	}
	ee := make([]string, 0)
	if len(worker.Config.HmacSecret) > 0 {
		ee = append(ee, acl)
	}
	if worker.Config.BadgerKeyFile != "" {
		ee = append(ee, encAtRest)
		ee = append(ee, encBackupRestore)
		ee = append(ee, encExport)
	} else {
		ee = append(ee, backupRestore)
	}
	return ee
}
