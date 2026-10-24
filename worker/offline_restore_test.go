/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/badger/v4/options"
	"github.com/dgraph-io/dgraph/v25/posting"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/x"
)

// writeTestBackup writes a full backup of group 1 to backupDir using the production
// BackupProcessor. It stores one value posting for every (namespace, uid) pair under the
// predicate "name" and returns the backup's read timestamp.
func writeTestBackup(t *testing.T, backupDir string, nsUids map[uint64][]uint64) uint64 {
	t.Helper()
	const commitTs, readTs = 5, 10

	db, err := badger.OpenManaged(badger.DefaultOptions(t.TempDir()).WithLogger(nil))
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()

	val, err := proto.Marshal(&pb.PostingList{Postings: []*pb.Posting{{
		Uid: 0, Value: []byte("node"), ValType: pb.Posting_STRING, PostingType: pb.Posting_VALUE,
	}}})
	require.NoError(t, err)

	var preds []string
	txn := db.NewTransactionAt(commitTs, true)
	for ns, uids := range nsUids {
		attr := x.NamespaceAttr(ns, "name")
		preds = append(preds, attr)
		for _, uid := range uids {
			key := x.DataKey(attr, uid)
			require.NoError(t, txn.SetEntry(badger.NewEntry(key, val).WithMeta(posting.BitCompletePosting)))
		}
	}
	require.NoError(t, txn.CommitAt(commitTs, nil))

	unixTs := time.Now().UTC().Format("20060102.150405.000")
	req := &pb.BackupRequest{
		ReadTs:      readTs,
		GroupId:     1,
		UnixTs:      unixTs,
		Destination: backupDir,
		Predicates:  preds,
		ForceFull:   true,
	}
	bp := NewBackupProcessor(db, req)
	defer bp.Close()
	_, err = bp.WriteBackup(context.Background())
	require.NoError(t, err)

	m := &Manifest{
		ManifestBase: ManifestBase{
			Type:        "full",
			ReadTs:      readTs,
			BackupId:    "offline-restore-lease",
			BackupNum:   1,
			Version:     x.ManifestVersion,
			Path:        "dgraph." + unixTs,
			Compression: "snappy",
		},
		Groups: map[uint32][]string{1: preds},
	}
	require.NoError(t, bp.CompleteBackup(context.Background(), m))
	return readTs
}

// TestRunOfflineRestoreReportsMaxLease checks that the offline restore (the code path behind
// `dgraph restore`) reports the max UID and namespace present in the backup. `dgraph restore -z`
// uses these values to bump Zero's UID and namespace leases. If they come back as 0, Zero hands
// out UIDs that already belong to restored nodes.
func TestRunOfflineRestoreReportsMaxLease(t *testing.T) {
	oldBadger, oldTmp := x.WorkerConfig.Badger, x.WorkerConfig.TmpDir
	// BackupProcessor sizes its per-thread state from this; it must match the DB's stream goroutines.
	x.WorkerConfig.Badger.NumGoroutines = badger.DefaultOptions("").NumGoroutines
	x.WorkerConfig.TmpDir = t.TempDir()
	defer func() { x.WorkerConfig.Badger, x.WorkerConfig.TmpDir = oldBadger, oldTmp }()

	const maxUid, maxNs = uint64(0x2711), uint64(2)
	nsUids := map[uint64][]uint64{
		x.RootNamespace: {0x1, 0x2, 0x3, maxUid},
		maxNs:           {0x10, 0x11},
	}
	backupDir := filepath.Join(t.TempDir(), "backups")
	readTs := writeTestBackup(t, backupDir, nsUids)

	// Control: the map phase does see the max UID and namespace in the backup.
	mapRes, err := RunMapper(&pb.RestoreRequest{Location: backupDir, GroupId: 1, RestoreTs: 1}, t.TempDir())
	require.NoError(t, err)
	require.Equal(t, maxUid, mapRes.maxUid, "mapper max uid")
	require.Equal(t, maxNs, mapRes.maxNs, "mapper max namespace")

	result := RunOfflineRestore(filepath.Join(t.TempDir(), "p"), backupDir, "", "", nil, options.Snappy, 0)
	require.NoError(t, result.Err)
	require.Equal(t, readTs, result.Version)
	assert.GreaterOrEqual(t, result.MaxLeaseUid, maxUid,
		"offline restore must report the max uid in the backup so Zero's uid lease can be bumped")
	assert.GreaterOrEqual(t, result.MaxLeaseNsId, maxNs,
		"offline restore must report the max namespace in the backup so Zero's ns lease can be bumped")
}
