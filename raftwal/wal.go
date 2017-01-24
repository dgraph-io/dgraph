package raftwal

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/boltdb/bolt"
	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/dgraph-io/dgraph/x"
)

type Wal struct {
	wals *bolt.DB
	id   uint64
}

func Init(walStore *bolt.DB, id uint64) *Wal {
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

func (w *Wal) StoreSnapshot(gid uint32, s raftpb.Snapshot) error {
	if raft.IsEmptySnap(s) {
		return nil
	}
	data, err := s.Marshal()
	if err != nil {
		return x.Wrapf(err, "wal.Store: While marshal snapshot")
	}

	// Delete all entries before this snapshot to save disk space.
	start := w.entryKey(gid, 0, 0)
	last := w.entryKey(gid, s.Metadata.Term, s.Metadata.Index)

	err = w.wals.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("data"))
		c := b.Cursor()

		for k, _ := c.Seek(start); k != nil; k, _ = c.Next() {
			if bytes.Compare(k, last) > 0 {
				break
			}
			if err := b.Delete(k); err != nil {
				return err
			}
		}
		if err := b.Put(w.snapshotKey(gid), data); err != nil {
			return err
		}
		fmt.Printf("Writing snapshot to WAL: %+v\n", s)
		return nil
	})

	return x.Wrapf(err, "wal.Store: While Store Snapshot")
}

// Store stores the snapshot, hardstate and entries for a given RAFT group.
func (w *Wal) Store(gid uint32, h raftpb.HardState, es []raftpb.Entry) error {

	err := w.wals.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("data"))
		if !raft.IsEmptyHardState(h) {
			data, err := h.Marshal()
			if err != nil {
				return x.Wrapf(err, "wal.Store: While marshal hardstate")
			}
			if err = b.Put(w.hardStateKey(gid), data); err != nil {
				return err
			}
		}

		var t, i uint64
		for _, e := range es {
			t, i = e.Term, e.Index
			data, err := e.Marshal()
			if err != nil {
				return x.Wrapf(err, "wal.Store: While marshal entry")
			}
			k := w.entryKey(gid, e.Term, e.Index)
			if err = b.Put(k, data); err != nil {
				return err
			}
		}

		// If we get no entries, then the default value of t and i would be zero. That would
		// end up deleting all the previous valid raft entry logs. This check avoids that.
		if t > 0 || i > 0 {
			// When writing an Entry with Index i, any previously-persisted entries
			// with Index >= i must be discarded.
			start := w.entryKey(gid, t, i+1)
			prefix := w.prefix(gid)

			c := b.Cursor()
			for k, _ := c.Seek(start); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
				if err := b.Delete(k); err != nil {
					return err
				}
			}
		}
		return nil
	})

	return x.Wrapf(err, "wal.Store: While WriteBatch")
}

func (w *Wal) Snapshot(gid uint32) (snap raftpb.Snapshot, rerr error) {
	var v []byte
	w.wals.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("data"))
		v = b.Get(w.snapshotKey(gid))
		return nil
	})
	if v != nil {
		rerr = x.Wrapf(snap.Unmarshal(v), "while unmarshal snapshot")
	}
	return
}

func (w *Wal) HardState(gid uint32) (hd raftpb.HardState, rerr error) {
	var v []byte
	w.wals.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("data"))
		v = b.Get(w.hardStateKey(gid))
		return nil
	})
	if v != nil {
		rerr = x.Wrapf(hd.Unmarshal(v), "While unmarshal snapshot")
	}
	return
}

func (w *Wal) Entries(gid uint32, fromTerm, fromIndex uint64) (es []raftpb.Entry, rerr error) {
	start := w.entryKey(gid, fromTerm, fromIndex)
	prefix := w.prefix(gid)

	w.wals.View(func(tx *bolt.Tx) error {
		c := tx.Bucket([]byte("data")).Cursor()
		for k, v := c.Seek(start); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
			var e raftpb.Entry
			if err := e.Unmarshal(v); err != nil {
				return x.Wrapf(err, "While unmarshal raftpb.Entry")
			}
			es = append(es, e)
		}
		return nil
	})
	return
}
