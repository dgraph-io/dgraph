package raftwal

import (
	"encoding/binary"
	"fmt"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

type Wal struct {
	wals *store.Store
}

func Init(walStore *store.Store) *Wal {
	return &Wal{wals: walStore}
}

func snapshotKey(gid uint32) []byte {
	b := make([]byte, 6)
	copy(b[0:2], []byte("ss"))
	binary.BigEndian.PutUint32(b[2:6], gid)
	return b
}

func hardStateKey(gid uint32) []byte {
	b := make([]byte, 6)
	copy(b[0:2], []byte("hs"))
	binary.BigEndian.PutUint32(b[2:6], gid)
	return b
}

func entryKey(gid uint32, term, idx uint64) []byte {
	b := make([]byte, 20)
	binary.BigEndian.PutUint32(b[0:4], gid)
	binary.BigEndian.PutUint64(b[4:12], term)
	binary.BigEndian.PutUint64(b[12:20], idx)
	return b
}

// Store stores the snapshot, hardstate and entries for a given RAFT group.
func (w *Wal) Store(gid uint32, s raftpb.Snapshot, h raftpb.HardState, es []raftpb.Entry) error {
	b := w.wals.NewWriteBatch()
	defer b.Destroy()

	if !raft.IsEmptySnap(s) {
		data, err := s.Marshal()
		if err != nil {
			return x.Wrapf(err, "wal.Store: While marshal snapshot")
		}
		fmt.Printf("w.Store snapshot: %+v\n", s)
		b.Put(snapshotKey(gid), data)
	}

	if !raft.IsEmptyHardState(h) {
		data, err := h.Marshal()
		if err != nil {
			return x.Wrapf(err, "wal.Store: While marshal hardstate")
		}
		fmt.Printf("w.Store hardstate: %+v\n", h)
		b.Put(hardStateKey(gid), data)
	}

	var t, i uint64
	for _, e := range es {
		t, i = e.Term, e.Index
		data, err := e.Marshal()
		if err != nil {
			return x.Wrapf(err, "wal.Store: While marshal entry")
		}
		k := entryKey(gid, e.Term, e.Index)
		fmt.Printf("w.Store [%d, %d] key: %x\n", t, i, k)
		b.Put(k, data)
	}

	if t > 0 || i > 0 {
		// Delete all keys above this index.
		start := entryKey(gid, t, i+1)
		fmt.Printf("Deleting keys after [%d, %d]\n", t, i+1)
		itr := w.wals.NewIterator()
		prefix := make([]byte, 4)
		binary.BigEndian.PutUint32(prefix, gid)

		for itr.Seek(start); itr.ValidForPrefix(prefix); itr.Next() {
			fmt.Printf("w.Store Delete key: %x\n", itr.Key().Data())
			b.Delete(itr.Key().Data())
		}
	}
	if b.Count() > 0 {
		fmt.Printf("wal.Store: Batch has %d writes\n", b.Count())
	}

	err := w.wals.WriteBatch(b)
	return x.Wrapf(err, "wal.Store: While WriteBatch")
}

func (w *Wal) Snapshot(gid uint32) (snap raftpb.Snapshot, rerr error) {
	data, err := w.wals.Get(snapshotKey(gid))
	if err != nil {
		return snap, x.Wrapf(err, "While getting snapshot")
	}
	rerr = x.Wrapf(snap.Unmarshal(data), "While unmarshal snapshot")
	return
}

func (w *Wal) HardState(gid uint32) (hd raftpb.HardState, rerr error) {
	data, err := w.wals.Get(hardStateKey(gid))
	if err != nil {
		return hd, x.Wrapf(err, "While getting hardstate")
	}
	rerr = x.Wrapf(hd.Unmarshal(data), "While unmarshal hardstate")
	return
}

func (w *Wal) Entries(gid uint32, fromTerm, fromIndex uint64) (es []raftpb.Entry, rerr error) {
	start := entryKey(gid, fromTerm, fromIndex)
	itr := w.wals.NewIterator()
	prefix := make([]byte, 4)
	binary.BigEndian.PutUint32(prefix, gid)

	for itr.SeekToFirst(); itr.Valid(); itr.Next() {
		fmt.Printf("Key: %x\n", itr.Key().Data())
	}

	for itr.Seek(start); itr.ValidForPrefix(prefix); itr.Next() {
		fmt.Printf("Entries Key: %v\n", itr.Key().Data())
		data := itr.Value().Data()
		var e raftpb.Entry
		if err := e.Unmarshal(data); err != nil {
			return es, x.Wrapf(err, "While unmarshal raftpb.Entry")
		}
		es = append(es, e)
	}
	return
}
