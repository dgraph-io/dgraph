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

package restore

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/dgraph-io/dgraph/ee/backup"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
)

func runRestore() error {
	if opt.pdir == "" {
		return x.Errorf("Must specify posting dir with -p")
	}
	if opt.loc == "" {
		return x.Errorf("Must specify a backup source with --loc")
	}

	req := &backup.Request{
		Backup: &pb.BackupRequest{Source: opt.loc},
	}
	f, err := req.OpenLocation(opt.loc)
	if err != nil {
		return err
	}

	bo := badger.DefaultOptions
	bo.SyncWrites = false
	bo.TableLoadingMode = options.MemoryMap
	bo.ValueThreshold = 1 << 10
	bo.NumVersionsToKeep = math.MaxInt32
	bo.Dir = opt.pdir
	bo.ValueDir = opt.pdir
	db, err := badger.OpenManaged(bo)
	if err != nil {
		return err
	}
	defer db.Close()

	writer := x.NewTxnWriter(db)
	writer.BlindWrite = true

	var (
		kvs pb.KVS
		bb  bytes.Buffer
		sz  uint64
		cnt int
	)
	kvs.Kv = make([]*pb.KV, 0, 1000)

	start := time.Now()
	fmt.Printf("Restore starting: %q\n", opt.loc)
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
			if cnt%100000 == 0 {
				fmt.Printf("--- writing %d keys\n", cnt)
			}
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
	fmt.Printf("Loaded %d keys in %s\n", cnt, time.Since(start).Round(time.Second))

	return nil
}
