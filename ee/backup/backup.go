/*
 * Copyright 2018 Dgraph Labs, Inc. All rights reserved.
 *
 */

package backup

import (
	"context"
	"math"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/worker/stream"
	"github.com/golang/glog"
)

type Worker struct {
	ReadTs  uint64
	GroupId uint32
	SeqTs   string
	DB      *badger.DB
}

func (w *Worker) Process(ctx context.Context) error {
	c, err := w.newWriter()
	if err != nil {
		glog.Errorf("Backup error while creating writer: %s\n", err)
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

	if err := sl.Orchestrate(ctx, "Backup", math.MaxUint64); err != nil {
		return err
	}
	return nil
}
