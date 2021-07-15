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

package worker

import (
	"context"
	"sync"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

func ProcessRestoreRequest(ctx context.Context, req *pb.RestoreRequest, wg *sync.WaitGroup) error {
	glog.Warningf("Restore failed: %v", x.ErrNotSupported)
	return x.ErrNotSupported
}

// Restore implements the Worker interface.
func (w *grpcWorker) Restore(ctx context.Context, req *pb.RestoreRequest) (*pb.Status, error) {
	glog.Warningf("Restore failed: %v", x.ErrNotSupported)
	return &pb.Status{}, x.ErrNotSupported
}

func handleRestoreProposal(ctx context.Context, req *pb.RestoreRequest, pidx uint64) error {
<<<<<<< HEAD
=======
	if req == nil {
		return errors.Errorf("nil restore request")
	}

	if req.IncrementalFrom == 1 {
		return errors.Errorf("Incremental restore must not include full backup")
	}

	// Clean up the cluster if it is a full backup restore.
	if req.IncrementalFrom == 0 {
		// Drop all the current data. This also cancels all existing transactions.
		dropProposal := pb.Proposal{
			Mutations: &pb.Mutations{
				GroupId: req.GroupId,
				StartTs: req.RestoreTs,
				DropOp:  pb.Mutations_ALL,
			},
		}
		if err := groups().Node.applyMutations(ctx, &dropProposal); err != nil {
			return err
		}
	}

	// TODO: after the drop, the tablets for the predicates stored in this group's
	// backup could be in a different group. The tablets need to be moved.

	// Reset tablets and set correct tablets to match the restored backup.
	creds := &x.MinioCredentials{
		AccessKey:    req.AccessKey,
		SecretKey:    req.SecretKey,
		SessionToken: req.SessionToken,
		Anonymous:    req.Anonymous,
	}
	uri, err := url.Parse(req.Location)
	if err != nil {
		return errors.Wrapf(err, "cannot parse backup location")
	}
	handler, err := NewUriHandler(uri, creds)
	if err != nil {
		return errors.Wrapf(err, "cannot create backup handler")
	}

	manifests, err := getManifestsToRestore(handler, uri, req)
	if err != nil {
		return errors.Wrapf(err, "cannot get backup manifests")
	}
	if len(manifests) == 0 {
		return errors.Errorf("no backup manifests found at location %s", req.Location)
	}

	lastManifest := manifests[0]
	restorePreds, ok := lastManifest.Groups[req.GroupId]

	if !ok {
		return errors.Errorf("backup manifest does not contain information for group ID %d",
			req.GroupId)
	}
	for _, pred := range restorePreds {
		// Force the tablet to be moved to this group, even if it's currently being served
		// by another group.
		if tablet, err := groups().ForceTablet(pred); err != nil {
			return errors.Wrapf(err, "cannot create tablet for restored predicate %s", pred)
		} else if tablet.GetGroupId() != req.GroupId {
			return errors.Errorf("cannot assign tablet for pred %s to group %d", pred, req.GroupId)
		}
	}

	mapDir, err := ioutil.TempDir(x.WorkerConfig.TmpDir, "restore-map")
	x.Check(err)
	defer os.RemoveAll(mapDir)
	glog.Infof("Created temporary map directory: %s\n", mapDir)

	// Map the backup.
	mapRes, err := RunMapper(req, mapDir)
	if err != nil {
		return errors.Wrapf(err, "Failed to map the backup files")
	}
	glog.Infof("Backup map phase is complete. Map result is: %+v\n", mapRes)

	sw := pstore.NewStreamWriter()
	defer sw.Cancel()

	prepareForReduce := func() error {
		if req.IncrementalFrom == 0 {
			return sw.Prepare()
		}
		// If there is a drop all in between the last restored backup and the incremental backups
		// then drop everything before restoring incremental backups.
		if mapRes.shouldDropAll {
			if err := pstore.DropAll(); err != nil {
				return errors.Wrap(err, "failed to reduce incremental restore map")
			}
		}

		dropAttrs := [][]byte{x.SchemaPrefix(), x.TypePrefix()}
		for ns := range mapRes.dropNs {
			prefix := x.DataPrefix(ns)
			dropAttrs = append(dropAttrs, prefix)
		}
		for attr := range mapRes.dropAttr {
			dropAttrs = append(dropAttrs, x.PredicatePrefix(attr))
		}

		// Any predicate which is currently in the state but not in the latest manifest should
		// be dropped. It is possible that the tablet would have been moved in between the last
		// restored backup and the incremental backups being restored.
		clusterPreds := schema.State().Predicates()
		validPreds := make(map[string]struct{})
		for _, pred := range restorePreds {
			validPreds[pred] = struct{}{}
		}
		for _, pred := range clusterPreds {
			if _, ok := validPreds[pred]; !ok {
				dropAttrs = append(dropAttrs, x.PredicatePrefix(pred))
			}
		}
		if err := pstore.DropPrefixBlocking(dropAttrs...); err != nil {
			return errors.Wrap(err, "failed to reduce incremental restore map")
		}
		if err := sw.PrepareIncremental(); err != nil {
			return errors.Wrapf(err, "while preparing DB")
		}
		return nil
	}

	if err := prepareForReduce(); err != nil {
		return errors.Wrap(err, "while preparing for reduce phase")
	}
	if err := RunReducer(sw, mapDir); err != nil {
		return errors.Wrap(err, "failed to reduce restore map")
	}
	if err := sw.Flush(); err != nil {
		return errors.Wrap(err, "while stream writer flush")
	}

	// Bump the UID and NsId lease after restore.
	if err := bumpLease(ctx, mapRes); err != nil {
		return errors.Wrap(err, "While bumping the leases after restore")
	}

	// Load schema back.
	if err := schema.LoadFromDb(); err != nil {
		return errors.Wrapf(err, "cannot load schema after restore")
	}

	// Reset gql schema only when the restore is not partial, so that after this restore the cluster
	// can be in non-draining mode and hence gqlSchema can be lazy loaded.
	glog.Info("reseting local gql schema store")
	if !req.IsPartial {
		ResetGQLSchemaStore()
	}
	ResetAclCache()

	// Propose a snapshot immediately after all the work is done to prevent the restore
	// from being replayed.
	go func(idx uint64) {
		n := groups().Node
		if !n.AmLeader() {
			return
		}
		if err := n.Applied.WaitForMark(context.Background(), idx); err != nil {
			glog.Errorf("Error waiting for mark for index %d: %+v", idx, err)
			return
		}
		if err := n.proposeSnapshot(); err != nil {
			glog.Errorf("cannot propose snapshot after processing restore proposal %+v", err)
		}
	}(pidx)

	// Update the membership state to re-compute the group checksums.
	if err := UpdateMembershipState(ctx); err != nil {
		return errors.Wrapf(err, "cannot update membership state after restore")
	}
>>>>>>> 9759fb038... fix(ACL): The Acl cache should be updated on restart and restore (#7926)
	return nil
}
