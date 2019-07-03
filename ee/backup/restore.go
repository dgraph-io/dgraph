// +build !oss

/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package backup

import (
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"path/filepath"

	"github.com/dgraph-io/badger"
	bpb "github.com/dgraph-io/badger/pb"
	"github.com/dgraph-io/badger/options"
	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
)

// RunRestore calls badger.Load and tries to load data into a new DB.
func RunRestore(pdir, location, backupId string) (uint64, error) {
	// Scan location for backup files and load them. Each file represents a node group,
	// and we create a new p dir for each.
	return Load(location, backupId, func(r io.Reader, groupId int) error {
		dir := filepath.Join(pdir, fmt.Sprintf("p%d", groupId))
		db, err := badger.OpenManaged(badger.DefaultOptions(dir).
			WithSyncWrites(true).
			WithTableLoadingMode(options.MemoryMap).
			WithValueThreshold(1 << 10).
			WithNumVersionsToKeep(math.MaxInt32))
		if err != nil {
			return err
		}
		defer db.Close()
		fmt.Printf("Restoring groupId: %d\n", groupId)
		if !pathExist(dir) {
			fmt.Println("Creating new db:", dir)
		}
		gzReader, err := gzip.NewReader(r)
		if err != nil {
			return nil
		}
		return loadFromBackup(db, gzReader, 16)
	})
}

// loadFromBackup reads the backup, converts the keys and values to the required format,
// and loads them to the given badger DB.
func loadFromBackup(db *badger.DB, r io.Reader, maxPendingWrites int) error {
	br := bufio.NewReaderSize(r, 16<<10)
	unmarshalBuf := make([]byte, 1<<10)

	loader := db.NewKVLoader(maxPendingWrites)
	for {
		var sz uint64
		err := binary.Read(br, binary.LittleEndian, &sz)
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		if cap(unmarshalBuf) < int(sz) {
			unmarshalBuf = make([]byte, sz)
		}

		if _, err = io.ReadFull(br, unmarshalBuf[:sz]); err != nil {
			return err
		}

		list := &bpb.KVList{}
		if err := list.Unmarshal(unmarshalBuf[:sz]); err != nil {
			return err
		}

		for _, kv := range list.Kv {
			fmt.Printf("backup key at restore point: %s", hex.Dump(kv.Key))

			if len(kv.GetUserMeta()) != 1 {
				return errors.Errorf(
					"Unexpected meta: %v for key: %s", kv.UserMeta, hex.Dump(kv.Key))
			}

			var restoreKey []byte
			var restoreVal []byte
			switch kv.GetUserMeta()[0]{
			case posting.BitEmptyPosting, posting.BitCompletePosting, posting.BitDeltaPosting:
				backupKey := &pb.BackupKey{}
				if err := backupKey.Unmarshal(kv.Key); err != nil {
					return errors.Wrapf(err, "while reading backup key %s", hex.Dump(kv.Key))
				}
				restoreKey = x.FromBackupKey(backupKey)

				var err error
				backupPl := &pb.BackupPostingList{}
				if err := backupPl.Unmarshal(kv.Value); err != nil {
					return errors.Wrapf(err, "while reading backup posting list")
				}
				restoreVal, err = posting.FromBackupPostingList(backupPl).Marshal()
				if err != nil {
					return errors.Wrapf(err, "while converting backup posting list")
				}

			case posting.BitSchemaPosting:
				restoreKey = kv.Key
				restoreVal = kv.Value

			default:
				return errors.Errorf(
					"Unexpected meta %d for key %s", kv.UserMeta[0], hex.Dump(kv.Key))
			}

			kv.Key = restoreKey
			kv.Value = restoreVal
			if err := loader.Set(kv); err != nil {
				return err
			}

			// // Update nextTxnTs, memtable stores this
			// // timestamp in badger head when flushed.
			// if kv.Version >= db.orc.nextTxnTs {
			// 	db.orc.nextTxnTs = kv.Version + 1
			// }
		}
	}

	if err := loader.Finish(); err != nil {
		return err
	}
	// db.orc.txnMark.Done(db.orc.nextTxnTs - 1)
	return nil
}
