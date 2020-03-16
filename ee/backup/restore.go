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
	"os"
	"path/filepath"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	bpb "github.com/dgraph-io/badger/v2/pb"
	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
)

// RunRestore calls badger.Load and tries to load data into a new DB.
func RunRestore(pdir, location, backupId string) LoadResult {
	// Create the pdir if it doesn't exist.
	if err := os.MkdirAll(pdir, 0700); err != nil {
		return LoadResult{0, 0, err}
	}

	// Scan location for backup files and load them. Each file represents a node group,
	// and we create a new p dir for each.
	return Load(location, backupId,
		func(r io.Reader, groupId int, preds predicateSet) (uint64, error) {

			dir := filepath.Join(pdir, fmt.Sprintf("p%d", groupId))
			db, err := badger.OpenManaged(badger.DefaultOptions(dir).
				WithSyncWrites(false).
				WithTableLoadingMode(options.MemoryMap).
				WithValueThreshold(1 << 10).
				WithNumVersionsToKeep(math.MaxInt32))
			if err != nil {
				return 0, err
			}
			defer db.Close()
			fmt.Printf("Restoring groupId: %d\n", groupId)
			if !pathExist(dir) {
				fmt.Println("Creating new db:", dir)
			}
			gzReader, err := gzip.NewReader(r)
			if err != nil {
				return 0, nil
			}
			maxUid, err := loadFromBackup(db, gzReader, preds)
			if err != nil {
				return 0, err
			}

			return maxUid, x.WriteGroupIdFile(dir, uint32(groupId))
		})
}

// loadFromBackup reads the backup, converts the keys and values to the required format,
// and loads them to the given badger DB. The set of predicates is used to avoid restoring
// values from predicates no longer assigned to this group.
func loadFromBackup(db *badger.DB, r io.Reader, preds predicateSet) (uint64, error) {
	br := bufio.NewReaderSize(r, 16<<10)
	unmarshalBuf := make([]byte, 1<<10)

	loader := db.NewKVLoader(16)
	var maxUid uint64
	for {
		var sz uint64
		err := binary.Read(br, binary.LittleEndian, &sz)
		if err == io.EOF {
			break
		} else if err != nil {
			return 0, err
		}

		if cap(unmarshalBuf) < int(sz) {
			unmarshalBuf = make([]byte, sz)
		}

		if _, err = io.ReadFull(br, unmarshalBuf[:sz]); err != nil {
			return 0, err
		}

		list := &bpb.KVList{}
		if err := list.Unmarshal(unmarshalBuf[:sz]); err != nil {
			return 0, err
		}

		for _, kv := range list.Kv {
			if len(kv.GetUserMeta()) != 1 {
				return 0, errors.Errorf(
					"Unexpected meta: %v for key: %s", kv.UserMeta, hex.Dump(kv.Key))
			}

			restoreKey, err := fromBackupKey(kv.Key)
			if err != nil {
				return 0, err
			}

			// Filter keys using the preds set. Do not do this filtering for type keys
			// as they are meant to be in every group and their Attr value does not
			// match a predicate name.
			parsedKey, err := x.Parse(restoreKey)
			if err != nil {
				return 0, errors.Wrapf(err, "could not parse key %s", hex.Dump(restoreKey))
			}
			if _, ok := preds[parsedKey.Attr]; !parsedKey.IsType() && !ok {
				continue
			}

			// Update the max id that has been seen while restoring this backup.
			if parsedKey.Uid > maxUid {
				maxUid = parsedKey.Uid
			}

			switch kv.GetUserMeta()[0] {
			case posting.BitEmptyPosting, posting.BitCompletePosting, posting.BitDeltaPosting:
				backupPl := &pb.BackupPostingList{}
				if err := backupPl.Unmarshal(kv.Value); err != nil {
					return 0, errors.Wrapf(err, "while reading backup posting list")
				}
				pl := posting.FromBackupPostingList(backupPl)
				shouldSplit := pl.Size() >= (1<<20)/2 && len(pl.Pack.Blocks) > 1

				if !shouldSplit || parsedKey.HasStartUid || len(pl.GetSplits()) > 0 {
					// This covers two cases.
					// 1. The list is not big enough to be split.
					// 2. This key is storing part of a multi-part list. Write each individual
					// part without rolling the key first. This part is here for backwards
					// compatibility. New backups are not affected because there was a change
					// to roll up lists into a single one.
					restoreVal, err := pl.Marshal()
					if err != nil {
						return 0, errors.Wrapf(err, "while converting backup posting list")
					}
					kv.Key = restoreKey
					kv.Value = restoreVal
					if err := loader.Set(kv); err != nil {
						return 0, err
					}
				} else {
					// This is a complete list. It should be rolled up to avoid writing
					// a list that is too big to be read back from disk.
					l := posting.NewList(restoreKey, pl, kv.Version)
					kvs, err := l.Rollup()
					if err != nil {
						// TODO: wrap errors in this file for easier debugging.
						return 0, err
					}
					for _, kv := range kvs {
						if err := loader.Set(kv); err != nil {
							return 0, err
						}
					}
				}

			case posting.BitSchemaPosting:
				// Schema and type keys are not stored in an intermediate format so their
				// value can be written as is.
				kv.Key = restoreKey
				if err := loader.Set(kv); err != nil {
					return 0, err
				}

			default:
				return 0, errors.Errorf(
					"Unexpected meta %d for key %s", kv.UserMeta[0], hex.Dump(kv.Key))
			}
		}
	}

	if err := loader.Finish(); err != nil {
		return 0, err
	}

	return maxUid, nil
}

func fromBackupKey(key []byte) ([]byte, error) {
	backupKey := &pb.BackupKey{}
	if err := backupKey.Unmarshal(key); err != nil {
		return nil, errors.Wrapf(err, "while reading backup key %s", hex.Dump(key))
	}
	return x.FromBackupKey(backupKey), nil
}
