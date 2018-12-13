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
	"path/filepath"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/dgraph-io/dgraph/ee/backup"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
)

func runRestore() error {
	bo := badger.DefaultOptions
	bo.SyncWrites = false
	bo.TableLoadingMode = options.MemoryMap
	bo.ValueThreshold = 1 << 10
	bo.NumVersionsToKeep = math.MaxInt32

	var num int
	return backup.Load(opt.location, func(reader io.Reader, name string) error {
		bo := bo
		bo.Dir = filepath.Join(opt.pdir, fmt.Sprintf("p%d", num))
		bo.ValueDir = bo.Dir
		db, err := badger.OpenManaged(bo)
		if err != nil {
			return err
		}
		defer db.Close()
		fmt.Println("--- Creating new db:", bo.Dir)

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

		// track progress
		tick := time.NewTicker(time.Second)
		done := make(chan struct{})
		if x.Config.DebugMode {
			go func() {
				for {
					select {
					case <-done:
						return
					case now := <-tick.C:
						fmt.Printf("... Time elapsed: %s, keys loaded: %d, speed: %d keys/s\n",
							now.Sub(start).Round(time.Second), cnt,
							int64(float64(cnt)/time.Since(start).Seconds()))
					}
				}
			}()
		}

		fmt.Println("--- Loading:", name)
		for {
			err = binary.Read(reader, binary.LittleEndian, &sz)
			if err == io.EOF {
				break
			} else if err != nil {
				return err
			}

			e := &pb.KV{}
			n, err := bb.ReadFrom(io.LimitReader(reader, int64(sz)))
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
		num++
		tick.Stop()
		done <- struct{}{}
		fmt.Printf("--- Loaded %d keys in %s\n", cnt, time.Since(start).Round(time.Second))

		return nil
	})
}
