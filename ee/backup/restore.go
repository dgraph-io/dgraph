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
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"math"
	"sync"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

func (r *Request) Restore(ctx context.Context) error {
	f, err := r.OpenLocation(r.Backup.Source)
	if err != nil {
		return err
	}

	entries := make([]*pb.KV, 0, 1000)
	start := time.Now()

	var (
		bb  bytes.Buffer
		sz  uint64
		cnt int
	)
	for {
		err = binary.Read(f, binary.LittleEndian, &sz)
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		e := &pb.KV{}
		n, err := bb.ReadFrom(io.LimitReader(f, int64(sz)))
		if err != nil {
			return err
		}
		if n != int64(sz) {
			return x.Errorf("Restore failed read. Expected %d bytes but got %d instead.", sz, n)
		}
		if err = e.Unmarshal((&bb).Bytes()); err != nil {
			return err
		}
		bb.Reset()
		entries = append(entries, e)
		cnt++
		if cnt%1000 == 0 {
			if err := setKeyValues(r.DB, entries); err != nil {
				return err
			}
			entries = entries[:0]
			if cnt%100000 == 0 {
				glog.V(3).Infof("--- writing %d keys", cnt)
			}
		}
	}
	if len(entries) > 0 {
		if err := setKeyValues(r.DB, entries); err != nil {
			return err
		}
	}
	glog.Infof("Loaded %d keys in %s\n", cnt, time.Since(start).Round(time.Second))

	return nil
}

func setKeyValues(pstore *badger.DB, kvs []*pb.KV) error {
	var wg sync.WaitGroup
	cerr := make(chan error, 1)
	wg.Add(len(kvs))
	for _, kv := range kvs {
		txn := pstore.NewTransactionAt(math.MaxUint64, true)
		if err := txn.SetWithMeta(kv.Key, kv.Val, kv.UserMeta[0]); err != nil {
			return err
		}
		err := txn.CommitAt(kv.Version, func(err error) {
			if err != nil {
				select {
				case cerr <- err:
				default:
				}
			}
			wg.Done()
		})
		if err != nil {
			return err
		}
		txn.Discard()
	}
	wg.Wait()
	select {
	case err := <-cerr:
		return x.Errorf("Error while writing: %s", err)
	default:
		return nil
	}
}
