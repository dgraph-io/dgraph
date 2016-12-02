package raftwal

import (
	"encoding/binary"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

type Wal struct {
	wals *store.Store
	id   uint64
}

func Init(walStore *store.Store, id uint64) *Wal {
	return &Wal{wals: walStore, id: id}
}

func (w *Wal) snapshotKey(gid uint32) []byte {
	b := make([]byte, 14)
	binary.BigEndian.PutUint64(b[0:8], w.id)
	copy(b[8:10], []byte("ss"))
	binary.BigEndian.PutUint32(b[10:14], gid)
	return b
}

func (w *Wal) hardStateKey(gid uint32) []byte {
	b := make([]byte, 14)
	binary.BigEndian.PutUint64(b[0:8], w.id)
	copy(b[8:10], []byte("hs"))
	binary.BigEndian.PutUint32(b[10:14], gid)
	return b
}

func (w *Wal) entryKey(gid uint32, term, idx uint64) []byte {
	b := make([]byte, 28)
	binary.BigEndian.PutUint64(b[0:8], w.id)
	binary.BigEndian.PutUint32(b[8:12], gid)
	binary.BigEndian.PutUint64(b[12:20], term)
	binary.BigEndian.PutUint64(b[20:28], idx)
	return b
}

func (w *Wal) prefix(gid uint32) []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint64(b[0:8], w.id)
	binary.BigEndian.PutUint32(b[8:12], gid)
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
		b.Put(w.snapshotKey(gid), data)
	}

	if !raft.IsEmptyHardState(h) {
		data, err := h.Marshal()
		if err != nil {
			return x.Wrapf(err, "wal.Store: While marshal hardstate")
		}
		b.Put(w.hardStateKey(gid), data)
	}

	var t, i uint64
	for _, e := range es {
		t, i = e.Term, e.Index
		data, err := e.Marshal()
		if err != nil {
			return x.Wrapf(err, "wal.Store: While marshal entry")
		}
		k := w.entryKey(gid, e.Term, e.Index)
		b.Put(k, data)
	}

	// If we get no entries, then the default value of t and i would be zero. That would
	// end up deleting all the previous valid raft entry logs. This check avoids that.
	if t > 0 || i > 0 {
		// Delete all keys above this index.
		start := w.entryKey(gid, t, i+1)
		prefix := w.prefix(gid)
		itr := w.wals.NewIterator()

		for itr.Seek(start); itr.ValidForPrefix(prefix); itr.Next() {
			b.Delete(itr.Key().Data())
		}
	}

	err := w.wals.WriteBatch(b)
	return x.Wrapf(err, "wal.Store: While WriteBatch")
}

func (w *Wal) Snapshot(gid uint32) (snap raftpb.Snapshot, rerr error) {
	slice, err := w.wals.Get(w.snapshotKey(gid))
	if err != nil || slice == nil {
		return snap, x.Wrapf(err, "While getting snapshot")
	}
	rerr = x.Wrapf(snap.Unmarshal(slice.Data()), "While unmarshal snapshot")
	slice.Free()
	return
}

func (w *Wal) HardState(gid uint32) (hd raftpb.HardState, rerr error) {
	slice, err := w.wals.Get(w.hardStateKey(gid))
	if err != nil || slice == nil {
		return hd, x.Wrapf(err, "While getting hardstate")
	}
	rerr = x.Wrapf(hd.Unmarshal(slice.Data()), "While unmarshal hardstate")
	slice.Free()
	return
}

func (w *Wal) Entries(gid uint32, fromTerm, fromIndex uint64) (es []raftpb.Entry, rerr error) {
	start := w.entryKey(gid, fromTerm, fromIndex)
	prefix := w.prefix(gid)
	itr := w.wals.NewIterator()

	for itr.Seek(start); itr.ValidForPrefix(prefix); itr.Next() {
		data := itr.Value().Data()
		var e raftpb.Entry
		if err := e.Unmarshal(data); err != nil {
			return es, x.Wrapf(err, "While unmarshal raftpb.Entry")
		}
		es = append(es, e)
	}
	return
}
