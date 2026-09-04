/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package posting

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/dgraph-io/badger/v4"
	bpb "github.com/dgraph-io/badger/v4/pb"
	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/codec"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/x"
	"github.com/dgraph-io/ristretto/v2/z"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// Tests share pstore; do not run in parallel.
type splitCleanupFixture struct {
	db   *badger.DB
	opt  badger.Options
	key  []byte
	attr string
}

// Create real splits at the default threshold.
func (f *splitCleanupFixture) seedNative(t *testing.T) ([]uint64, []uint64) {
	t.Helper()
	var uids []uint64
	var edges []*pb.DirectedEdge
	for uid := uint64(1); uid <= 1024; uid++ {
		uids = append(uids, uid)
		edges = append(edges, &pb.DirectedEdge{Op: pb.DirectedEdge_SET, ValueId: uid,
			Facets: []*api.Facet{{Key: "label", ValType: api.Facet_STRING, Value: bytes.Repeat([]byte("x"), 2048)}},
		})
	}
	f.mutate(t, 1, 10, edges...)
	l := f.read(t, 15, uids)
	kvs, err := l.Rollup(nil, 15)
	require.NoError(t, err)
	f.write(t, kvs)
	f.reopen(t)
	l = f.read(t, 15, uids)
	splits := append([]uint64(nil), l.PartSplits()...)
	require.GreaterOrEqual(t, len(splits), 2)
	t.Logf("native split creation: edges=%d, parent splits=%v", len(uids), splits)
	return uids, splits
}

func (f *splitCleanupFixture) assertBackup(t *testing.T, ts uint64, expected []uint64) {
	t.Helper()
	l := f.read(t, ts, expected)
	buf := z.NewBuffer(10<<10, "TestSplitCleanupBackup")
	defer func() { require.NoError(t, buf.Release()) }()
	var bl pb.BackupPostingList
	_, err := l.ToBackupPostingList(&bl, nil, buf)
	require.NoError(t, err)
	pl := FromBackupPostingList(&bl)
	defer codec.FreePack(pl.Pack)
	require.Empty(t, pl.Splits)
	require.Equal(t, expected, append([]uint64{}, codec.Decode(pl.Pack, 0)...))
}

func TestRollupNativeSplitCleanup(t *testing.T) {
	for _, whole := range []bool{false, true} {
		t.Run(fmt.Sprintf("star=%t", whole), func(t *testing.T) {
			f := newSplitCleanupFixture(t, "data")
			original, splits := f.seedNative(t)
			expected := []uint64{}
			var edges []*pb.DirectedEdge
			if whole {
				edges = append(edges, &pb.DirectedEdge{Op: pb.DirectedEdge_DEL, Value: []byte(x.Star)})
			} else {
				for _, uid := range original {
					if uid >= splits[len(splits)/2] {
						edges = append(edges, &pb.DirectedEdge{Op: pb.DirectedEdge_DEL, ValueId: uid})
					} else {
						expected = append(expected, uid)
					}
				}
			}
			f.mutate(t, 19, 20, edges...)
			l := f.read(t, 30, expected)
			kvs, err := l.Rollup(nil, 30)
			require.NoError(t, err)
			f.write(t, kvs)
			f.reopen(t)
			f.db.SetDiscardTs(15)
			require.NoError(t, f.db.Flatten(1))
			f.read(t, 15, original)
			f.assertBackup(t, 15, original)
			f.assertBackup(t, 30, expected)
			latest := f.read(t, 30, expected)
			live := make(map[uint64]bool)
			for _, start := range latest.PartSplits() {
				live[start] = true
			}
			f.write(t, kvs)
			f.reopen(t)
			f.db.SetDiscardTs(100)
			require.NoError(t, f.db.Flatten(1))
			f.read(t, 100, expected)
			r := f.db.NewTransactionAt(100, false)
			defer r.Discard()
			removed := 0
			for _, start := range splits {
				if live[start] {
					continue
				}
				removed++
				key, err := x.SplitKey(f.key, start)
				require.NoError(t, err)
				it := r.NewKeyIterator(key, badger.IteratorOptions{AllVersions: true})
				var payload int64
				for it.Rewind(); it.Valid(); it.Next() {
					payload += it.Item().ValueSize()
				}
				it.Close()
				t.Logf("native removed split=%d retained payload=%d", start, payload)
				require.Zero(t, payload, "native split %d retains obsolete payload", start)
			}
			require.Positive(t, removed)
		})
	}
}

func TestRollupSplitCleanupHistoryAndReinsert(t *testing.T) {
	for _, whole := range []bool{false, true} {
		t.Run(fmt.Sprintf("star=%t", whole), func(t *testing.T) {
			f := newSplitCleanupFixture(t, "data")
			original, _ := f.seedNative(t)
			oldList := f.read(t, 15, original)
			expected := []uint64{}
			var deletes []*pb.DirectedEdge
			if whole {
				deletes = append(deletes, &pb.DirectedEdge{Op: pb.DirectedEdge_DEL, Value: []byte(x.Star)})
			} else {
				for _, uid := range original {
					if uid >= 513 {
						deletes = append(deletes, &pb.DirectedEdge{Op: pb.DirectedEdge_DEL, ValueId: uid})
					} else {
						expected = append(expected, uid)
					}
				}
			}
			f.mutate(t, 19, 20, deletes...)
			l := f.read(t, 30, expected)
			cleanup, err := l.Rollup(nil, 30)
			require.NoError(t, err)

			// Commit a newer mutation before persisting the rollup.
			var inserts []*pb.DirectedEdge
			for _, uid := range original {
				inserts = append(inserts, &pb.DirectedEdge{Op: pb.DirectedEdge_SET, ValueId: uid,
					Facets: []*api.Facet{{Key: "label", ValType: api.Facet_STRING, Value: bytes.Repeat([]byte("y"), 2048)}},
				})
			}
			f.mutate(t, 31, 40, inserts...)

			// Read the old parent during rollup writes.
			readErrors := make(chan error, 1)
			go func() {
				for i := 0; i < 100; i++ {
					var got []uint64
					err := oldList.Iterate(15, 0, func(p *pb.Posting) error {
						got = append(got, p.Uid)
						if len(p.Facets) != 1 || !bytes.Equal(p.Facets[0].Value, bytes.Repeat([]byte("x"), 2048)) {
							return fmt.Errorf("historical facet changed for UID %d", p.Uid)
						}
						return nil
					})
					if err != nil {
						readErrors <- err
						return
					}
					if !reflect.DeepEqual(original, got) {
						readErrors <- fmt.Errorf("historical UIDs changed")
						return
					}
				}
				readErrors <- nil
			}()
			f.write(t, cleanup)
			require.NoError(t, <-readErrors)
			f.read(t, 15, original)
			f.read(t, 30, expected)
			l = f.read(t, 40, original)
			latest, err := l.Rollup(nil, 45)
			require.NoError(t, err)
			f.write(t, latest)
			f.write(t, cleanup) // Replay the older rollup after reinsertion.
			f.reopen(t)
			f.db.SetDiscardTs(15)
			require.NoError(t, f.db.Flatten(1))
			f.read(t, 15, original)
			f.read(t, 19, original)
			for _, ts := range []uint64{20, 21, 30, 39} {
				f.read(t, ts, expected)
			}
			for _, ts := range []uint64{40, 41, 45} {
				l := f.read(t, ts, original)
				require.NoError(t, l.Iterate(ts, 0, func(p *pb.Posting) error {
					require.Len(t, p.Facets, 1)
					require.Equal(t, bytes.Repeat([]byte("y"), 2048), p.Facets[0].Value)
					return nil
				}))
			}
			f.assertBackup(t, 15, original)
			f.assertBackup(t, 30, expected)
			f.assertBackup(t, 45, original)
			f.write(t, latest)
			f.reopen(t)
			f.db.SetDiscardTs(100)
			require.NoError(t, f.db.Flatten(1))
			f.read(t, 100, original)
			t.Log("historical reads, concurrent old-parent reads, backup conversion, restart, and reinsertion preserved")
		})
	}
}

func newSplitCleanupFixture(t *testing.T, kind string) *splitCleanupFixture {
	t.Helper()
	f := &splitCleanupFixture{
		opt: badger.DefaultOptions(t.TempDir()).WithLogger(nil).
			WithNumVersionsToKeep(math.MaxInt32).WithNumCompactors(0).
			WithNumLevelZeroTables(1).WithNumLevelZeroTablesStall(10),
		attr: x.AttrInRootNamespace("split-cleanup"),
	}
	f.key = x.DataKey(f.attr, 1)
	switch kind {
	case "reverse":
		f.key = x.ReverseKey(f.attr, 1)
	case "index":
		f.key = x.IndexKey(f.attr, "token")
	}
	var err error
	f.db, err = badger.OpenManaged(f.opt)
	require.NoError(t, err)
	original := pstore
	pstore = f.db
	t.Cleanup(func() {
		pstore = original
		require.NoError(t, f.db.Close())
	})
	return f
}

func (f *splitCleanupFixture) reopen(t *testing.T) {
	t.Helper()
	require.NoError(t, f.db.Close())
	var err error
	f.db, err = badger.OpenManaged(f.opt)
	require.NoError(t, err)
	pstore = f.db
}

func (f *splitCleanupFixture) write(t *testing.T, kvs []*bpb.KV) {
	t.Helper()
	w := NewTxnWriter(f.db)
	require.NoError(t, w.Write(&bpb.KVList{Kv: kvs}))
	require.NoError(t, w.Flush())
}

func (f *splitCleanupFixture) seed(t *testing.T) {
	t.Helper()
	kv := MarshalPostingList(&pb.PostingList{Splits: []uint64{1, 100}}, nil)
	kv.Key, kv.Version = f.key, 10
	kvs := []*bpb.KV{kv}
	for start, uids := range map[uint64][]uint64{1: {2, 3}, 100: {100, 101}} {
		key, err := x.SplitKey(f.key, start)
		require.NoError(t, err)
		kv := MarshalPostingList(&pb.PostingList{Pack: codec.Encode(uids, blockSize)}, nil)
		kv.Key, kv.Version = key, 10
		kvs = append(kvs, kv)
	}
	f.write(t, kvs)
	f.reopen(t)
}

func (f *splitCleanupFixture) read(t *testing.T, ts uint64, expected []uint64) *List {
	t.Helper()
	l, err := readPostingListFromDisk(f.key, f.db, ts)
	require.NoError(t, err)
	require.Equal(t, expected, listToArray(t, 0, l, ts), "readTs=%d", ts)
	return l
}

func (f *splitCleanupFixture) mutate(t *testing.T, start, commit uint64, edges ...*pb.DirectedEdge) {
	t.Helper()
	l, err := readPostingListFromDisk(f.key, f.db, start)
	require.NoError(t, err)
	txn := NewTxn(start)
	for _, edge := range edges {
		edge.Entity, edge.Attr = 1, f.attr
		require.NoError(t, l.addMutation(context.Background(), txn, edge))
	}
	delta := l.mutationMap.get(start)
	require.NotNil(t, delta)
	require.NoError(t, l.commitMutation(start, commit))
	value, err := proto.Marshal(delta)
	require.NoError(t, err)
	w := NewTxnWriter(f.db)
	require.NoError(t, w.SetAt(f.key, value, BitDeltaPosting, commit))
	require.NoError(t, w.Flush())
}

func TestRollupSplitCleanupParentTimestampBoundary(t *testing.T) {
	f := newSplitCleanupFixture(t, "data")
	f.seed(t)
	f.mutate(t, 19, 20, &pb.DirectedEdge{Op: pb.DirectedEdge_DEL, Value: []byte(x.Star)})
	l := f.read(t, 30, []uint64{})
	kvs, err := l.Rollup(nil, 30)
	require.NoError(t, err)
	f.write(t, kvs)
	f.reopen(t)
	// The old parent is still visible at 20; its replacement is written at 21.
	f.db.SetDiscardTs(20)
	require.NoError(t, f.db.Flatten(1))
	f.read(t, 20, []uint64{})
	f.read(t, 21, []uint64{})
}

func TestRollupReclaimsRemovedSplitParts(t *testing.T) {
	for _, kind := range []string{"data", "reverse", "index"} {
		for _, whole := range []bool{false, true} {
			for _, rollupTs := range []uint64{30, math.MaxUint64} {
				t.Run(fmt.Sprintf("%s/star=%t/readTs=%d", kind, whole, rollupTs), func(t *testing.T) {
					f := newSplitCleanupFixture(t, kind)
					f.seed(t)
					original := []uint64{2, 3, 100, 101}
					f.read(t, 15, original)
					expected := []uint64{2, 3}
					removed := []uint64{100}
					if whole {
						f.mutate(t, 19, 20, &pb.DirectedEdge{Op: pb.DirectedEdge_DEL, Value: []byte(x.Star)})
						expected = []uint64{}
						removed = []uint64{1, 100}
					} else {
						f.mutate(t, 19, 20,
							&pb.DirectedEdge{Op: pb.DirectedEdge_DEL, ValueId: 100},
							&pb.DirectedEdge{Op: pb.DirectedEdge_DEL, ValueId: 101})
					}
					l := f.read(t, 30, expected)
					kvs, err := l.Rollup(nil, rollupTs)
					require.NoError(t, err)
					f.write(t, kvs)
					f.reopen(t)

					f.db.SetDiscardTs(15)
					require.NoError(t, f.db.Flatten(1))
					f.read(t, 15, original)
					f.read(t, 19, original)
					for _, ts := range []uint64{20, 21, 30} {
						f.read(t, ts, expected)
					}

					// Create overlapping SSTs for compaction.
					f.write(t, kvs)
					f.reopen(t)
					f.db.SetDiscardTs(100)
					require.NoError(t, f.db.Flatten(1))
					f.read(t, 100, expected)

					r := f.db.NewTransactionAt(100, false)
					defer r.Discard()
					for _, start := range removed {
						key, err := x.SplitKey(f.key, start)
						require.NoError(t, err)
						it := r.NewKeyIterator(key, badger.IteratorOptions{AllVersions: true})
						var payload int64
						for it.Rewind(); it.Valid(); it.Next() {
							item := it.Item()
							payload += item.ValueSize()
							t.Logf("removed split=%d version=%d meta=%d bytes=%d", start,
								item.Version(), item.UserMeta(), item.ValueSize())
						}
						it.Close()
						require.Zero(t, payload, "unreferenced split %d retains old payload after compaction", start)
					}
				})
			}
		}
	}
}
