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
	"time"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

func (r *Request) Restore(ctx context.Context) error {
	f, err := r.OpenLocation(r.Backup.Source)
	if err != nil {
		return err
	}

	writer := x.NewTxnWriter(r.DB)
	writer.BlindWrite = true

	var (
		kvs pb.KVS
		bb  bytes.Buffer
		sz  uint64
		cnt int
	)
	kvs.Kv = make([]*pb.KV, 0, 1000)

	start := time.Now()
	glog.Infof("Restore starting: %q", r.Backup.Source)
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
		kvs.Kv = append(kvs.Kv, e)
		kvs.Done = false
		cnt++
		if cnt%1000 == 0 {
			if err := writer.Send(&kvs); err != nil {
				return err
			}
			kvs.Kv = kvs.Kv[:0]
			kvs.Done = true
		}
	}
	if !kvs.Done {
		if err := writer.Send(&kvs); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	glog.Infof("Loaded %d keys in %s", cnt, time.Since(start).Round(time.Second))

	return nil
}
