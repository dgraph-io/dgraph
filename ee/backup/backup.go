// +build !oss

/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
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
	DB     *badger.DB // Badger pstore managed by this node.
	Sizex  uint64     // approximate upload size
	Backup *pb.BackupRequest
}

// Process uses the request values to create a stream writer then hand off the data
// retrieval to stream.Orchestrate. The writer will create all the fd's needed to
// collect the data and later move to the target.
// Returns errors on failure, nil on success.
func (r *Request) Process(ctx context.Context) error {
	f, err := r.OpenLocation(r.Backup.Target)
	if err != nil {
		return err
	}

	sl := stream.Lists{Stream: f, DB: r.DB}
	sl.ChooseKeyFunc = nil
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

	glog.V(2).Infof("Backup started ...")
	if err = sl.Orchestrate(ctx, "Backup:", r.Backup.ReadTs); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	glog.Infof("Backup complete: group %d at %d", r.Backup.GroupId, r.Backup.ReadTs)

	return nil
}
