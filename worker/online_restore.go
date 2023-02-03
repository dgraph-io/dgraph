//go:build !oss
// +build !oss

/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package worker

import (
	"context"
	"fmt"
	"io/ioutil"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/dgraph-io/badger/v3"
	"github.com/dgraph-io/badger/v3/options"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/ee"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
)

const (
	errRestoreProposal = "cannot propose restore request"
)

// verifyRequest verifies that the manifest satisfies the requirements to process the given
// restore request.
func verifyRequest(h UriHandler, uri *url.URL, req *pb.RestoreRequest,
	currentGroups []uint32) error {

	manifests, err := getManifestsToRestore(h, uri, req)
	if err != nil {
		return errors.Wrapf(err, "while retrieving manifests")
	}
	if len(manifests) == 0 {
		return errors.Errorf("No backups with the specified backup ID %s", req.GetBackupId())
	}

	// TODO(Ahsan): Do we need to verify the manifests again here?
	if err := verifyManifests(manifests); err != nil {
		return err
	}

	lastManifest := manifests[0]
	if len(currentGroups) != len(lastManifest.Groups) {
		return errors.Errorf("groups in cluster and latest backup manifest differ")
	}

	for _, group := range currentGroups {
		if _, ok := lastManifest.Groups[group]; !ok {
			return errors.Errorf("groups in cluster and latest backup manifest differ")
		}
	}
	return nil
}

// VerifyBackup will access the backup location and verify that the specified backup can
// be restored to the cluster.
func VerifyBackup(req *pb.RestoreRequest, creds *x.MinioCredentials, currentGroups []uint32) error {
	uri, err := url.Parse(req.GetLocation())
	if err != nil {
		return err
	}

	h, err := NewUriHandler(uri, creds)
	if err != nil {
		return errors.Wrap(err, "VerifyBackup")
	}

	return verifyRequest(h, uri, req, currentGroups)
}

// ProcessRestoreRequest verifies the backup data and sends a restore proposal to each group.
func ProcessRestoreRequest(ctx context.Context, req *pb.RestoreRequest, wg *sync.WaitGroup) error {
	if req == nil {
		return errors.Errorf("restore request cannot be nil")
	}

	if err := UpdateMembershipState(ctx); err != nil {
		return errors.Wrapf(err, "cannot update membership state before restore")
	}
	memState := GetMembershipState()

	currentGroups := make([]uint32, 0)
	for gid := range memState.GetGroups() {
		currentGroups = append(currentGroups, gid)
	}

	creds := x.MinioCredentials{
		AccessKey:    req.AccessKey,
		SecretKey:    req.SecretKey,
		SessionToken: req.SessionToken,
		Anonymous:    req.Anonymous,
	}
	if err := VerifyBackup(req, &creds, currentGroups); err != nil {
		return errors.Wrapf(err, "failed to verify backup")
	}
	if err := FillRestoreCredentials(req.Location, req); err != nil {
		return errors.Wrapf(err, "cannot fill restore proposal with the right credentials")
	}

	// This check if any restore operation running on the node.
	// Operation initiated on other nodes doesn't have record in the record tracker.
	// This keeps track if there is an already running restore operation return the error.
	// IMP: This introduces few corner cases.
	// Like two concurrent restore operation on different nodes.
	// Considering Restore as admin operation, solving all those complexities has low gains
	// than to sacrifice the simplicity.
	isRestoreRunning := func() bool {
		tasks := GetOngoingTasks()
		for _, t := range tasks {
			if t == opRestore.String() {
				return true
			}
		}
		return false
	}
	if isRestoreRunning() {
		return errors.Errorf("another restore operation is already running. " +
			"Please retry later.")
	}

	req.RestoreTs = State.GetTimestamp(false)

	// TODO: prevent partial restores when proposeRestoreOrSend only sends the restore
	// request to a subset of groups.
	errCh := make(chan error, len(currentGroups))
	for _, gid := range currentGroups {
		reqCopy := proto.Clone(req).(*pb.RestoreRequest)
		reqCopy.GroupId = gid
		wg.Add(1)
		go func() {
			errCh <- tryRestoreProposal(ctx, reqCopy)
		}()
	}

	go func() {
		for range currentGroups {
			if err := <-errCh; err != nil {
				glog.Errorf("Error while restoring %v", err)
			}
			wg.Done()
		}
	}()

	return nil
}

func proposeRestoreOrSend(ctx context.Context, req *pb.RestoreRequest) error {
	if groups().ServesGroup(req.GetGroupId()) && groups().Node.AmLeader() {
		_, err := (&grpcWorker{}).Restore(ctx, req)
		return err
	}

	pl := groups().Leader(req.GetGroupId())
	if pl == nil {
		return conn.ErrNoConnection
	}
	con := pl.Get()
	c := pb.NewWorkerClient(con)

	_, err := c.Restore(ctx, req)
	return err
}

func retriableRestoreError(err error) bool {
	switch {
	case err == conn.ErrNoConnection:
		// Try to recover from temporary connection issues.
		return true
	case strings.Contains(err.Error(), "Raft isn't initialized yet"):
		// Try to recover if raft has not been initialized.
		return true
	case strings.Contains(err.Error(), errRestoreProposal):
		// Do not try to recover from other errors when sending the proposal.
		return false
	default:
		// Try to recover from other errors (e.g wrong group, waiting for timestamp, etc).
		return true
	}
}

func tryRestoreProposal(ctx context.Context, req *pb.RestoreRequest) error {
	var err error
	for i := 0; i < 10; i++ {
		err = proposeRestoreOrSend(ctx, req)
		if err == nil {
			return nil
		}

		if retriableRestoreError(err) {
			time.Sleep(time.Second)
			continue
		}
		return err
	}
	return err
}

// Restore implements the Worker interface.
func (w *grpcWorker) Restore(ctx context.Context, req *pb.RestoreRequest) (*pb.Status, error) {
	var emptyRes pb.Status
	if !groups().ServesGroup(req.GroupId) {
		return &emptyRes, errors.Errorf("this server doesn't serve group id: %v", req.GroupId)
	}

	// We should wait to ensure that we have seen all the updates until the StartTs
	// of this restore transaction.
	if err := posting.Oracle().WaitForTs(ctx, req.RestoreTs); err != nil {
		return nil, errors.Wrapf(err, "cannot wait for restore ts %d", req.RestoreTs)
	}

	err := groups().Node.proposeAndWait(ctx, &pb.Proposal{Restore: req})
	if err != nil {
		return &emptyRes, errors.Wrapf(err, errRestoreProposal)
	}

	return &emptyRes, nil
}

// TODO(DGRAPH-1232): Ensure all groups receive the restore proposal.
func handleRestoreProposal(ctx context.Context, req *pb.RestoreRequest, pidx uint64) error {
	if req == nil {
		return errors.Errorf("nil restore request")
	}

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
	preds, ok := lastManifest.Groups[req.GroupId]

	// Version is 0 if the backup was taken on an old version (v20.11).
	if lastManifest.Version == 0 {
		tmp := make([]string, 0, len(preds))
		for _, pred := range preds {
			tmp = append(tmp, x.GalaxyAttr(pred))
		}
		preds = tmp
	}

	if !ok {
		return errors.Errorf("backup manifest does not contain information for group ID %d",
			req.GroupId)
	}
	for _, pred := range preds {
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

	// Reduce the map to pstore using stream writer.
	sw := pstore.NewStreamWriter()
	if err := sw.Prepare(); err != nil {
		return errors.Wrapf(err, "while preparing DB")
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
	if err := schema.LoadFromDb(ctx); err != nil {
		return errors.Wrapf(err, "cannot load schema after restore")
	}

	ResetAclCache()

	// reset gql schema
	glog.Info("reseting local gql schema store")
	ResetGQLSchemaStore()

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
	return nil
}

func bumpLease(ctx context.Context, mr *mapResult) error {
	pl := groups().connToZeroLeader()
	if pl == nil {
		return errors.Errorf("cannot update lease due to no connection to zero leader")
	}

	zc := pb.NewZeroClient(pl.Get())
	bump := func(val uint64, typ pb.NumLeaseType) error {
		_, err := zc.AssignIds(ctx, &pb.Num{Val: val, Type: typ, Bump: true})
		if err != nil && strings.Contains(err.Error(), "Nothing to be leased") {
			return nil
		}
		return err
	}

	if err := bump(mr.maxUid, pb.Num_UID); err != nil {
		return errors.Wrapf(err, "cannot update max uid lease after restore.")
	}
	if err := bump(mr.maxNs, pb.Num_NS_ID); err != nil {
		return errors.Wrapf(err, "cannot update max namespace lease after restore.")
	}
	return nil
}

// create a config object from the request for use with enc package.
func getEncConfig(req *pb.RestoreRequest) (*viper.Viper, error) {
	config := viper.New()
	flags := &pflag.FlagSet{}
	ee.RegisterEncFlag(flags)
	if err := config.BindPFlags(flags); err != nil {
		return nil, errors.Wrapf(err, "bad config bind")
	}

	// Copy from the request.
	config.Set("encryption", ee.BuildEncFlag(req.EncryptionKeyFile))

	vaultBuilder := new(strings.Builder)
	if req.VaultRoleidFile != "" {
		fmt.Fprintf(vaultBuilder, "role-id-file=%s;", req.VaultRoleidFile)
	}
	if req.VaultSecretidFile != "" {
		fmt.Fprintf(vaultBuilder, "secret-id-file=%s;", req.VaultSecretidFile)
	}
	if req.VaultAddr != "" {
		fmt.Fprintf(vaultBuilder, "addr=%s;", req.VaultAddr)
	}
	if req.VaultPath != "" {
		fmt.Fprintf(vaultBuilder, "path=%s;", req.VaultPath)
	}
	if req.VaultField != "" {
		fmt.Fprintf(vaultBuilder, "field=%s;", req.VaultField)
	}
	if req.VaultFormat != "" {
		fmt.Fprintf(vaultBuilder, "format=%s;", req.VaultFormat)
	}
	if vaultConfig := vaultBuilder.String(); vaultConfig != "" {
		config.Set("vault", vaultConfig)
	}

	return config, nil
}

func getCredentialsFromRestoreRequest(req *pb.RestoreRequest) *x.MinioCredentials {
	return &x.MinioCredentials{
		AccessKey:    req.AccessKey,
		SecretKey:    req.SecretKey,
		SessionToken: req.SessionToken,
		Anonymous:    req.Anonymous,
	}
}

// RunOfflineRestore creates required DBs and streams the backups to them.
func RunOfflineRestore(dir, location, backupId, keyFile string, key x.Sensitive,
	ctype options.CompressionType, clevel int) LoadResult {

	// Create the pdir if it doesn't exist.
	if err := os.MkdirAll(dir, 0700); err != nil {
		return LoadResult{Err: err}
	}

	uri, err := url.Parse(location)
	if err != nil {
		return LoadResult{Err: err}
	}

	h, err := NewUriHandler(uri, nil)
	if err != nil {
		return LoadResult{Err: errors.Errorf("Unsupported URI: %v", uri)}
	}
	manifest, err := GetLatestManifest(h, uri)
	if err != nil {
		return LoadResult{Err: errors.Wrapf(err, "cannot retrieve manifests")}
	}
	if len(keyFile) > 0 {
		key, err = ioutil.ReadFile(keyFile)
		if err != nil {
			return LoadResult{Err: errors.Wrapf(err, "RunRestore failed to read enc-key")}
		}
	}

	mapDir, err := ioutil.TempDir(x.WorkerConfig.TmpDir, "restore-map")
	x.Check(err)
	defer os.RemoveAll(mapDir)

	for gid := range manifest.Groups {
		req := &pb.RestoreRequest{
			Location:          location,
			GroupId:           gid,
			BackupId:          backupId,
			EncryptionKeyFile: keyFile,
			RestoreTs:         1,
		}
		if _, err := RunMapper(req, mapDir); err != nil {
			return LoadResult{Err: errors.Wrap(err, "RunRestore failed to map")}
		}
		pdir := filepath.Join(dir, fmt.Sprintf("p%d", gid))
		db, err := badger.OpenManaged(badger.DefaultOptions(pdir).
			WithCompression(ctype).
			WithZSTDCompressionLevel(clevel).
			WithSyncWrites(false).
			WithBlockCacheSize(100 * (1 << 20)).
			WithIndexCacheSize(100 * (1 << 20)).
			WithNumVersionsToKeep(math.MaxInt32).
			WithEncryptionKey(key).
			WithNamespaceOffset(x.NamespaceOffset))
		if err != nil {
			return LoadResult{Err: errors.Wrap(err, "RunRestore failed to open DB")}
		}
		defer db.Close()

		sw := db.NewStreamWriter()
		if err := sw.Prepare(); err != nil {
			return LoadResult{Err: errors.Wrap(err, "while preparing DB")}
		}
		if err := RunReducer(sw, mapDir); err != nil {
			return LoadResult{Err: errors.Wrap(err, "RunRestore failed to reduce")}
		}
		if err := sw.Flush(); err != nil {
			return LoadResult{Err: errors.Wrap(err, "while stream writer flush")}
		}
		if err := x.WriteGroupIdFile(pdir, uint32(gid)); err != nil {
			return LoadResult{Err: errors.Wrap(err, "RunRestore failed to write group id file")}
		}
	}
	// TODO: Fix this return value.
	return LoadResult{Version: manifest.ValidReadTs()}
}
