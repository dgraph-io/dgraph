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
	"net/http"

	"github.com/dgraph-io/dgraph/graphql/schema"

	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

func init() {
	http.Handle("/admin/backup", allowedMethodsHandler(allowedMethods{http.MethodPost: true},
		adminAuthHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			backupHandler(w, r)
		}))))
}

// backupHandler handles backup requests coming from the HTTP endpoint.
func backupHandler(w http.ResponseWriter, r *http.Request) {
	gqlReq := &schema.Request{
		Query: `
		mutation backup($input: BackupInput!) {
		  backup(input: $input) {
			response {
			  code
			}
		  }
		}`,
		Variables: map[string]interface{}{"input": map[string]interface{}{
			"destination":  r.FormValue("destination"),
			"accessKey":    r.FormValue("access_key"),
			"secretKey":    r.FormValue("secret_key"),
			"sessionToken": r.FormValue("session_token"),
			"anonymous":    r.FormValue("anonymous") == "true",
			"forceFull":    r.FormValue("force_full") == "true",
		}},
	}
	glog.Infof("gqlReq %+v, r %+v adminServer %+v", gqlReq, r, adminServer)
	resp := resolveWithAdminServer(gqlReq, r, adminServer)
	if resp.Errors != nil {
		x.SetStatus(w, resp.Errors.Error(), "Backup failed.")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	x.Check2(w.Write([]byte(`{"code": "Success", "message": "Backup completed."}`)))
}
