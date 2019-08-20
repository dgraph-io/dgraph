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
	"net/url"
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
	if !worker.EnterpriseEnabled() {
		x.SetStatus(w, "You must enable enterprise features first. "+
			"Supply the appropriate license file to Dgraph Zero using the HTTP endpoint.",
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
	}

	// Read the manifests to get the right timestamp from which to start the backup.
	uri, err := url.Parse(req.Destination)
	if err != nil {
		return err
	}
	handler, err := backup.NewUriHandler(uri)
	if err != nil {
		return err
	}
	latestManifest, err := handler.GetLatestManifest(uri)
	if err != nil {
		return err
	}
	req.SinceTs = latestManifest.Since
	if forceFull {
		req.SinceTs = 0
	}

	// Update the membership state to get the latest mapping of groups to predicates.
	if err := worker.UpdateMembershipState(ctx); err != nil {
		return err
	}

	// Get the current membership state and parse it for easier processing.
	state := worker.GetMembershipState()
	var groups []uint32
	predMap := make(map[uint32][]string)
	for gid, group := range state.Groups {
		groups = append(groups, gid)
		predMap[gid] = make([]string, 0)
		for pred := range group.Tablets {
			predMap[gid] = append(predMap[gid], pred)
		}
	}

	glog.Infof("Created backup request: %s. Groups=%v\n", &req, groups)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, len(state.Groups))
	for _, gid := range groups {
		req := req
		req.GroupId = gid
		req.Predicates = predMap[gid]
		go func(req *pb.BackupRequest) {
			_, err := worker.BackupGroup(ctx, req)
			errCh <- err
		}(&req)
	}

	for range groups {
		if err := <-errCh; err != nil {
			glog.Errorf("Error received during backup: %v", err)
			return err
		}
	}

	m := backup.Manifest{Since: req.ReadTs, Groups: predMap}
	if req.SinceTs == 0 {
		m.Type = "full"
		m.BackupId = x.GetRandomName(1)
		m.BackupNum = 1
	} else {
		m.Type = "incremental"
		m.BackupId = latestManifest.BackupId
		m.BackupNum = latestManifest.BackupNum + 1
	}

	bp := &backup.Processor{Request: &req}
	return bp.CompleteBackup(ctx, &m)
}
