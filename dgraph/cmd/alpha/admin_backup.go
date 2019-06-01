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
	"time"

	"github.com/dgraph-io/dgraph/ee/backup"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"

	"github.com/golang/glog"
	"github.com/pkg/errors"
)

func init() {
	http.HandleFunc("/admin/backup", backupHandler)
}

// backupHandler handles backup requests coming from the HTTP endpoint.
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

	if err := processHttpBackupRequest(context.Background(), r); err != nil {
		x.SetStatus(w, err.Error(), "Backup failed.")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	x.Check2(w.Write([]byte(`{"code": "Success", "message": "Backup completed."}`)))
}

func processHttpBackupRequest(ctx context.Context, r *http.Request) error {
	destination := r.FormValue("destination")
	if destination == "" {
		return errors.Errorf("You must specify a 'destination' value")
	}

	accessKey := r.FormValue("access_key")
	secretKey := r.FormValue("secret_key")
	sessionToken := r.FormValue("session_token")
	anonymous := r.FormValue("anonymous") == "true"
	forceFull := r.FormValue("force_full") == "true"

	if err := x.HealthCheck(); err != nil {
		glog.Errorf("Backup canceled, not ready to accept requests: %s", err)
		return err
	}

	ts, err := worker.Timestamps(ctx, &pb.Num{ReadOnly: true})
	if err != nil {
		glog.Errorf("Unable to retrieve readonly timestamp for backup: %s", err)
		return err
	}

	req := pb.BackupRequest{
		ReadTs:       ts.ReadOnly,
		Destination:  destination,
		UnixTs:       time.Now().UTC().Format("20060102.150405"),
		AccessKey:    accessKey,
		SecretKey:    secretKey,
		SessionToken: sessionToken,
		Anonymous:    anonymous,
		// TODO(martinmr): Check if this field can be removed.
		ForceFull: forceFull,
	}
	m := backup.Manifest{Groups: worker.KnownGroups()}
	glog.Infof("Created backup request: %s. Groups=%v\n", &req, m.Groups)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, len(m.Groups))
	for _, gid := range m.Groups {
		req := req
		req.GroupId = gid
		go func(req *pb.BackupRequest) {
			res, err := worker.BackupGroup(ctx, req)
			errCh <- err

			// Update manifest if appropriate.
			m.Lock()
			if res.Since > m.Since {
				m.Since = res.Since
			}
			m.Unlock()
		}(&req)
	}

	for range m.Groups {
		if err := <-errCh; err != nil {
			glog.Errorf("Error received during backup: %v", err)
			return err
		}
	}

	br := &backup.Request{Backup: &req, Manifest: &m}
	return br.Complete(ctx)
}
