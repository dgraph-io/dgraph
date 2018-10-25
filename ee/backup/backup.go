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
	"github.com/dgraph-io/dgraph/x"
)

type Worker struct {
	ReadTs uint64
	DB     *badger.DB
}

type collector struct {
	kv []*pb.KV
}

func (c *collector) Send(kvs *pb.KVS) error {
	c.kv = append(c.kv, kvs.Kv...)
	return nil
}

func (w *Worker) Process(ctx context.Context) error {
	c := &collector{kv: make([]*pb.KV, 0, 100)}
	sl := stream.Lists{Stream: c, DB: w.DB}
	sl.ItemToKVFunc = func(key []byte, itr *badger.Iterator) (*pb.KV, error) {
		item := itr.Item()
		val, err := item.ValueCopy(nil)
		if err != nil {
			return nil, err
		}
		kv := &pb.KV{Key: item.KeyCopy(nil), Val: val, Version: item.Version()}
		itr.Next() // Just for fun.
		return kv, nil
	}

	// Test case 1. Retrieve everything.
	err := sl.Orchestrate(context.Background(), "Backup", math.MaxUint64)
	if err != nil {
		return err
	}

	m := make(map[string]int)
	for _, kv := range c.kv {
		pk := x.Parse(kv.Key)
		m[pk.Attr]++
	}
	// for pred, count := range m {
	// 	require.Equal(t, 100, count, "Count mismatch for pred: %s", pred)
	// }

	return nil
}
