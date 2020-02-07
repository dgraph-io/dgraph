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

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

func init() {
	http.HandleFunc("/admin/backup", backupHandler)
}

// backupHandler handles backup requests coming from the HTTP endpoint.
func backupHandler(w http.ResponseWriter, r *http.Request) {
	if !handlerInit(w, r, map[string]bool{
		http.MethodPost: true,
	}) {
		return
	}

	destination := r.FormValue("destination")
	accessKey := r.FormValue("access_key")
	secretKey := r.FormValue("secret_key")
	sessionToken := r.FormValue("session_token")
	anonymous := r.FormValue("anonymous") == "true"
	forceFull := r.FormValue("force_full") == "true"

	req := pb.BackupRequest{
		Destination:  destination,
		AccessKey:    accessKey,
		SecretKey:    secretKey,
		SessionToken: sessionToken,
		Anonymous:    anonymous,
	}

	if err := worker.ProcessBackupRequest(context.Background(), &req, forceFull); err != nil {
		x.SetStatus(w, err.Error(), "Backup failed.")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	x.Check2(w.Write([]byte(`{"code": "Success", "message": "Backup completed."}`)))
}
