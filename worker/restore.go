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

package worker

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
	"strconv"

	"github.com/dgraph-io/badger/v3"
	"github.com/dgraph-io/badger/v3/options"
	bpb "github.com/dgraph-io/badger/v3/pb"
	"github.com/golang/glog"
	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/ee/enc"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
)

// RunRestore calls badger.Load and tries to load data into a new DB.
func RunRestore(pdir, location, backupId string, key x.SensitiveByteSlice,
	ctype options.CompressionType, clevel int) LoadResult {
	// Create the pdir if it doesn't exist.
	if err := os.MkdirAll(pdir, 0700); err != nil {
		return LoadResult{Err: err}
	}

	var isOld bool
	// Scan location for backup files and load them. Each file represents a node group,
	// and we create a new p dir for each.
	result := LoadBackup(location, backupId, 0, nil,
		func(groupId uint32, in *loadBackupInput) (uint64, uint64, error) {

			dir := filepath.Join(pdir, fmt.Sprintf("p%d", groupId))
			r, err := enc.GetReader(key, in.r)
			if err != nil {
				return 0, 0, err
			}

			gzReader, err := gzip.NewReader(r)
			if err != nil {
				if len(key) != 0 {
					err = errors.Wrap(err,
						"Unable to read the backup. Ensure the encryption key is correct.")
				}
				return 0, 0, err
			}

			if !pathExist(dir) {
				fmt.Println("Creating new db:", dir)
			}
			// The badger DB should be opened only after creating the backup
			// file reader and verifying the encryption in the backup file.
			db, err := badger.OpenManaged(badger.DefaultOptions(dir).
				WithCompression(ctype).
				WithZSTDCompressionLevel(clevel).
				WithSyncWrites(false).
				WithBlockCacheSize(100 * (1 << 20)).
				WithIndexCacheSize(100 * (1 << 20)).
				WithNumVersionsToKeep(math.MaxInt32).
				WithEncryptionKey(key).
				WithNamespaceOffset(x.NamespaceOffset))
			if err != nil {
				return 0, 0, err
			}
			defer db.Close()
			maxUid, maxNsId, err := loadFromBackup(db, &loadBackupInput{
				r:              gzReader,
				restoreTs:      0,
				preds:          in.preds,
				dropOperations: in.dropOperations,
				isOld:          in.isOld,
			})
			if err != nil {
				return 0, 0, err
			}
			isOld = in.isOld
			return maxUid, maxNsId, x.WriteGroupIdFile(dir, uint32(groupId))
		})

	if result.Err == nil && isOld {
		// Group 1 contains all the internal predicates.
		dir := filepath.Join(pdir, fmt.Sprintf("p%d", 1))
		pstore, result.Err = badger.OpenManaged(badger.DefaultOptions(dir).
			WithCompression(ctype).
			WithZSTDCompressionLevel(clevel).
			WithSyncWrites(false).
			WithBlockCacheSize(100 * (1 << 20)).
			WithIndexCacheSize(100 * (1 << 20)).
			WithNumVersionsToKeep(math.MaxInt32).
			WithEncryptionKey(key))
		if result.Err != nil {
			return result
		}
		defer pstore.Close()
		result.Err = fixRestore()
	}
	return result
}

func getData(attr string, fn func(item *badger.Item) error) error {
	return pstore.View(func(txn *badger.Txn) error {
		attr = x.GalaxyAttr(attr)
		initKey := x.ParsedKey{
			Attr: attr,
		}
		prefix := initKey.DataPrefix()
		// We expect a single node for predicate like dgraph.cors. But we have seen bugs where there
		// have been more than one cors node. In that case, we pick up the one with higher UID.
		startKey := append(x.DataKey(attr, math.MaxUint64))

		itOpt := badger.DefaultIteratorOptions
		itOpt.AllVersions = true
		itOpt.Reverse = true
		itOpt.Prefix = prefix
		itr := txn.NewIterator(itOpt)
		defer itr.Close()
		for itr.Seek(startKey); itr.Valid(); itr.Next() {
			item := itr.Item()
			if item.UserMeta() != posting.BitCompletePosting {
				// We expect only complete posting list.
				return errors.Errorf("item does not contain a complete posting.")
			}
			if err := fn(item); err != nil {
				return err
			}
			break
		}
		return nil
	})
}

func updateGqlSchema(cors [][]byte) error {
	var entry *badger.Entry
	var version uint64
	err := getData("dgraph.graphql.schema", func(item *badger.Item) error {
		var err error
		entry.Key = item.KeyCopy(nil)
		entry.Value, err = item.ValueCopy(nil)
		if err != nil {
			return err
		}
		entry.UserMeta = item.UserMeta()
		entry.ExpiresAt = item.ExpiresAt()
		version = item.Version()
		return nil
	})
	if err != nil {
		return err
	}
	if entry == nil {
		// We haven't found any graphql schema. It is safe to ignore the CORS information.
		// This means CORS will be inferred as * .
		return nil
	}

	var corsBytes []byte
	corsBytes = append(corsBytes, []byte("\n\n\n# Below schema elements will only work for dgraph"+
		" versions >= 21.03. In older versions it will be ignored.")...)
	for _, val := range cors {
		corsBytes = append(corsBytes, []byte(fmt.Sprintf("\n# Dgraph.Allow-Origin \"%s\"",
			string(val)))...)
	}

	// Append the cors at the end of GraphQL schema.
	pl := pb.PostingList{}
	if err = pl.Unmarshal(entry.Value); err != nil {
		return err
	}
	if len(pl.Postings) != 1 {
		return errors.Errorf("Only one posting is expected in the graphql schema posting list "+
			"but got %d", len(pl.Postings))
	}
	pl.Postings[0].Value = append(pl.Postings[0].Value, corsBytes...)
	entry.Value, err = pl.Marshal()
	if err != nil {
		return err
	}

	txn := pstore.NewTransactionAt(math.MaxUint64, true)
	defer txn.Discard()
	if err = txn.SetEntry(entry); err != nil {
		return err
	}
	return txn.CommitAt(version, nil)
}

func getCors() ([][]byte, error) {
	var corsVals [][]byte
	err := getData("dgraph.cors", func(item *badger.Item) error {
		val, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		pl := pb.PostingList{}
		if err := pl.Unmarshal(val); err != nil {
			return err
		}
		for _, p := range pl.Postings {
			corsVals = append(corsVals, p.Value)
		}
		return nil
	})
	return corsVals, err
}

var depreciatedPreds = map[string]struct{}{
	"dgraph.cors":                      {},
	"dgraph.graphql.schema_created_at": {},
	"dgraph.graphql.schema_history":    {},
	"dgraph.graphql.p_sha256hash":      {},
}

var depreciatedTypes = map[string]struct{}{
	"dgraph.type.cors":       {},
	"dgraph.graphql.history": {},
}

func dropDepreciated() error {
	var prefixes [][]byte
	for pred := range depreciatedPreds {
		pred = x.GalaxyAttr(pred)
		prefixes = append(prefixes, x.SchemaKey(pred), x.PredicatePrefix(pred))
	}
	for typ := range depreciatedTypes {
		prefixes = append(prefixes, x.TypeKey(x.GalaxyAttr(typ)))
	}
	return pstore.DropPrefix(prefixes...)
}

// fixRestore removes the internal predicates from the restored backup. This also appends CORS from
// dgraph.cors to GraphQL schema.
func fixRestore() error {
	glog.Infof("Fixing restore from version < 21.03.")
	cors, err := getCors()
	if err != nil {
		return errors.Wrapf(err, "Error while getting cors from db.")
	}
	if err := updateGqlSchema(cors); err != nil {
		return errors.Wrapf(err, "Error while updating GraphQL schema.")
	}
	return errors.Wrapf(dropDepreciated(), "Error while dropping depreciated preds/types.")
}

type loadBackupInput struct {
	r              io.Reader
	restoreTs      uint64
	preds          predicateSet
	dropOperations []*pb.DropOperation
	isOld          bool
}

// loadFromBackup reads the backup, converts the keys and values to the required format,
// and loads them to the given badger DB. The set of predicates is used to avoid restoring
// values from predicates no longer assigned to this group.
// If restoreTs is greater than zero, the key-value pairs will be written with that timestamp.
// Otherwise, the original value is used.
// TODO(DGRAPH-1234): Check whether restoreTs can be removed.
func loadFromBackup(db *badger.DB, in *loadBackupInput) (uint64, uint64, error) {
	br := bufio.NewReaderSize(in.r, 16<<10)
	unmarshalBuf := make([]byte, 1<<10)

	// if there were any DROP operations that need to be applied before loading the backup into
	// the db, then apply them here
	if err := applyDropOperationsBeforeRestore(db, in.dropOperations); err != nil {
		return 0, 0, errors.Wrapf(err, "cannot apply DROP operations while loading backup")
	}

	// Delete schemas and types. Each backup file should have a complete copy of the schema.
	if err := db.DropPrefix([]byte{x.ByteSchema}); err != nil {
		return 0, 0, err
	}
	if err := db.DropPrefix([]byte{x.ByteType}); err != nil {
		return 0, 0, err
	}

	loader := db.NewKVLoader(16)
	var maxUid, maxNsId uint64
	for {
		var sz uint64
		err := binary.Read(br, binary.LittleEndian, &sz)
		if err == io.EOF {
			break
		} else if err != nil {
			return 0, 0, err
		}

		if cap(unmarshalBuf) < int(sz) {
			unmarshalBuf = make([]byte, sz)
		}

		if _, err = io.ReadFull(br, unmarshalBuf[:sz]); err != nil {
			return 0, 0, err
		}

		list := &bpb.KVList{}
		if err := list.Unmarshal(unmarshalBuf[:sz]); err != nil {
			return 0, 0, err
		}

		for _, kv := range list.Kv {
			if len(kv.GetUserMeta()) != 1 {
				return 0, 0, errors.Errorf(
					"Unexpected meta: %v for key: %s", kv.UserMeta, hex.Dump(kv.Key))
			}

			restoreKey, namespace, err := fromBackupKey(kv.Key)
			if err != nil {
				return 0, 0, err
			}

			// Filter keys using the preds set. Do not do this filtering for type keys
			// as they are meant to be in every group and their Attr value does not
			// match a predicate name.
			parsedKey, err := x.Parse(restoreKey)
			if err != nil {
				return 0, 0, errors.Wrapf(err, "could not parse key %s", hex.Dump(restoreKey))
			}
			if _, ok := in.preds[parsedKey.Attr]; !parsedKey.IsType() && !ok {
				continue
			}

			// Update the max uid and namespace id that has been seen while restoring this backup.
			maxUid = x.Max(maxUid, parsedKey.Uid)
			maxNsId = x.Max(maxNsId, namespace)

			// Override the version if requested. Should not be done for type and schema predicates,
			// which always have their version set to 1.
			if in.restoreTs > 0 && !parsedKey.IsSchema() && !parsedKey.IsType() {
				kv.Version = in.restoreTs
			}

			switch kv.GetUserMeta()[0] {
			case posting.BitEmptyPosting, posting.BitCompletePosting, posting.BitDeltaPosting:
				backupPl := &pb.BackupPostingList{}
				if err := backupPl.Unmarshal(kv.Value); err != nil {
					return 0, 0, errors.Wrapf(err, "while reading backup posting list")
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
					newKv := posting.MarshalPostingList(pl, nil)
					newKv.Key = restoreKey
					// Use the version of the KV before we marshalled the
					// posting list. The MarshalPostingList function returns KV
					// with a zero version.
					newKv.Version = kv.Version
					if err := loader.Set(newKv); err != nil {
						return 0, 0, err
					}
				} else {
					// This is a complete list. It should be rolled up to avoid writing
					// a list that is too big to be read back from disk.
					// Rollup will take ownership of the Pack and will free the memory.
					l := posting.NewList(restoreKey, pl, kv.Version)
					kvs, err := l.Rollup(nil)
					if err != nil {
						// TODO: wrap errors in this file for easier debugging.
						return 0, 0, err
					}
					for _, kv := range kvs {
						if err := loader.Set(kv); err != nil {
							return 0, 0, err
						}
					}
				}

			case posting.BitSchemaPosting:
				appendNamespace := func() error {
					// If the backup was taken on old version, we need to append the namespace to
					// the fields of TypeUpdate.
					var update pb.TypeUpdate
					if err := update.Unmarshal(kv.Value); err != nil {
						return err
					}
					for _, sch := range update.Fields {
						sch.Predicate = x.GalaxyAttr(sch.Predicate)
					}
					kv.Value, err = update.Marshal()
					return err
				}
				if in.isOld && parsedKey.IsType() {
					if err := appendNamespace(); err != nil {
						glog.Errorf("Unable to (un)marshal type: %+v. Err=%v\n", parsedKey, err)
						continue
					}
				}
				// Schema and type keys are not stored in an intermediate format so their
				// value can be written as is.
				kv.Key = restoreKey
				if err := loader.Set(kv); err != nil {
					return 0, 0, err
				}

			default:
				return 0, 0, errors.Errorf(
					"Unexpected meta %d for key %s", kv.UserMeta[0], hex.Dump(kv.Key))
			}
		}
	}

	if err := loader.Finish(); err != nil {
		return 0, 0, err
	}

	return maxUid, maxNsId, nil
}

func applyDropOperationsBeforeRestore(db *badger.DB, dropOperations []*pb.DropOperation) error {
	for _, operation := range dropOperations {
		switch operation.DropOp {
		case pb.DropOperation_ALL:
			return db.DropAll()
		case pb.DropOperation_DATA:
			return db.DropPrefix([]byte{x.DefaultPrefix})
		case pb.DropOperation_ATTR:
			return db.DropPrefix(x.PredicatePrefix(operation.DropValue))
		case pb.DropOperation_NS:
			ns, err := strconv.ParseUint(operation.DropValue, 0, 64)
			x.Check(err)
			return db.BanNamespace(ns)
		}
	}
	return nil
}

func fromBackupKey(key []byte) ([]byte, uint64, error) {
	backupKey := &pb.BackupKey{}
	if err := backupKey.Unmarshal(key); err != nil {
		return nil, 0, errors.Wrapf(err, "while reading backup key %s", hex.Dump(key))
	}
	return x.FromBackupKey(backupKey), backupKey.Namespace, nil
}
