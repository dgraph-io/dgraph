/*
 * Copyright 2018 Dgraph Labs, Inc. All rights reserved.
 *
 */

package backup

import (
	"context"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/stream"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

// Request has all the information needed to perform a backup.
type Request struct {
	ReadTs    uint64     // Timestamp to read at.
	GroupId   uint32     // The group ID of this node.
	UnixTs    string     // Sequence Ts to label the backup file(s) at the target.
	TargetURI string     // The intended location as URI.
	DB        *badger.DB // Badger pstore managed by this node.
}

// Process uses the request values to create a stream writer then hand off the data
// retrieval to stream.Orchestrate. The writer will create all the fd's needed to
// collect the data and later move to the target.
// Returns errors on failure, nil on success.
func (r *Request) Process(ctx context.Context) error {
	w, err := newWriter(r)
	if err != nil {
		return err
	}
	sl := stream.Lists{Stream: w, DB: r.DB}
	sl.ChooseKeyFunc = func(_ *badger.Item) bool { return true }
	sl.ItemToKVFunc = func(key []byte, itr *badger.Iterator) (*pb.KV, error) {
		item := itr.Item()
		pk := x.Parse(key)
		if pk.IsSchema() {
			val, err := item.ValueCopy(nil)
			if err != nil {
				return nil, err
			}
			kv := &pb.KV{
				Key:      key,
				Val:      val,
				UserMeta: []byte{item.UserMeta()},
				Version:  item.Version(),
			}
			return kv, nil
		}
		l, err := posting.ReadPostingList(key, itr)
		if err != nil {
			return nil, err
		}
		return l.MarshalToKv()
	}

	glog.V(3).Infof("Backup started ...")
	if err = sl.Orchestrate(ctx, "Backup", r.ReadTs); err != nil {
		return err
	}
	glog.V(3).Infof("Backup finishing ...")
	if err = w.close(); err != nil {
		return err
	}
	glog.Infof("Backup complete: group %d at %d", r.GroupId, r.ReadTs)

	return nil
}
