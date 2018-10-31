/*
 * Copyright 2018 Dgraph Labs, Inc. All rights reserved.
 *
 */

package backup

import (
	"context"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/worker/stream"
	"github.com/golang/glog"
)

// Worker has all the information needed to perform a backup.
type Worker struct {
	ReadTs    uint64     // Timestamp to read at.
	GroupId   uint32     // The group ID of this node.
	SeqTs     string     // Sequence data to label backup at the target.
	TargetURI string     // The intended location as URI.
	DB        *badger.DB // Badger pstore managed by this node.
}

// Process uses the worker values to create a stream writer then hand off the data
// retrieval to stream.Orchestrate. The writer will create all the fd's needed to
// collect the data and later move to the target.
// Returns errors on failure, nil on success.
func (w *Worker) Process(ctx context.Context) error {
	c, err := newWriter(w)
	if err != nil {
		return err
	}
	sl := stream.Lists{Stream: c, DB: w.DB}
	sl.ChooseKeyFunc = func(_ *badger.Item) bool { return true }
	sl.ItemToKVFunc = func(key []byte, itr *badger.Iterator) (*pb.KV, error) {
		item := itr.Item()
		val, err := item.ValueCopy(nil)
		if err != nil {
			return nil, err
		}
		kv := &pb.KV{Key: item.KeyCopy(nil), Val: val, Version: item.Version()}
		return kv, nil
	}

	glog.Infof("Backup started ...")
	if err = sl.Orchestrate(ctx, "Backup", w.ReadTs); err != nil {
		return err
	}
	glog.Infof("Backup saving ...")
	if err = c.save(); err != nil {
		return err
	}
	glog.Infof("Backup complete: group %d @ %d", w.GroupId, w.ReadTs)

	return nil
}
