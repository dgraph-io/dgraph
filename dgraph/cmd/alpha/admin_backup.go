// +build !oss

/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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

package alpha

import (
	"context"
	"net/http"

	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

func backupHandler(w http.ResponseWriter, r *http.Request) {
	if !handlerInit(w, r, http.MethodPost) {
		return
	}
	if !Alpha.Conf.GetBool("enterprise_features") {
		x.SetStatus(w,
			"You must enable Dgraph enterprise features first. "+
				"Restart Dgraph Alpha with --enterprise_features",
			"Backup failed.")
		return
	}

	destination := r.FormValue("destination")
	if destination == "" {
		x.SetStatus(w, "You must specify a 'destination' value", "Backup failed.")
		return
	}
	if err := worker.BackupOverNetwork(context.Background(), destination); err != nil {
		x.SetStatus(w, err.Error(), "Backup failed.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	x.Check2(w.Write([]byte(`{"code": "Success", "message": "Backup completed."}`)))
}

func init() {
	http.HandleFunc("/admin/backup", backupHandler)
}
