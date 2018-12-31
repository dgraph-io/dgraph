/*
 * Copyright 2017 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package badger

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"sync"

	"github.com/dgraph-io/badger/pb"
	"github.com/dgraph-io/badger/y"
)

// Backup is a wrapper function over Stream.Backup to generate full and incremental backups of the
// DB. For more control over how many goroutines are used to generate the backup, or if you wish to
// backup only a certain range of keys, use Stream.Backup directly.
func (db *DB) Backup(w io.Writer, since uint64) (uint64, error) {
	stream := db.NewStream()
	stream.LogPrefix = "DB.Backup"
	return stream.Backup(w, since)
}

// Backup dumps a protobuf-encoded list of all entries in the database into the
// given writer, that are newer than the specified version. It returns a
// timestamp indicating when the entries were dumped which can be passed into a
// later invocation to generate an incremental dump, of entries that have been
// added/modified since the last invocation of Stream.Backup().
//
// This can be used to backup the data in a database at a given point in time.
func (stream *Stream) Backup(w io.Writer, since uint64) (uint64, error) {
	stream.KeyToList = func(key []byte, itr *Iterator) (*pb.KVList, error) {
		list := &pb.KVList{}
		for ; itr.Valid(); itr.Next() {
			item := itr.Item()
			if !bytes.Equal(item.Key(), key) {
				return list, nil
			}
			if item.Version() < since {
				// Ignore versions less than given timestamp, or skip older
				// versions of the given key.
				return list, nil
			}

			var valCopy []byte
			if !item.IsDeletedOrExpired() {
				// No need to copy value, if item is deleted or expired.
				var err error
				valCopy, err = item.ValueCopy(nil)
				if err != nil {
					Errorf("Key [%x, %d]. Error while fetching value [%v]\n",
						item.Key(), item.Version(), err)
					return nil, err
				}
			}

			// clear txn bits
			meta := item.meta &^ (bitTxn | bitFinTxn)
			kv := &pb.KV{
				Key:       item.KeyCopy(nil),
				Value:     valCopy,
				UserMeta:  []byte{item.UserMeta()},
				Version:   item.Version(),
				ExpiresAt: item.ExpiresAt(),
				Meta:      []byte{meta},
			}
			list.Kv = append(list.Kv, kv)

			switch {
			case item.DiscardEarlierVersions():
				// If we need to discard earlier versions of this item, add a delete
				// marker just below the current version.
				list.Kv = append(list.Kv, &pb.KV{
					Key:     item.KeyCopy(nil),
					Version: item.Version() - 1,
					Meta:    []byte{bitDelete},
				})
				return list, nil

			case item.IsDeletedOrExpired():
				return list, nil
			}
		}
		return list, nil
	}

	var maxVersion uint64
	stream.Send = func(list *pb.KVList) error {
		for _, kv := range list.Kv {
			if maxVersion < kv.Version {
				maxVersion = kv.Version
			}
			if err := writeTo(kv, w); err != nil {
				return err
			}
		}
		return nil
	}

	if err := stream.Orchestrate(context.Background()); err != nil {
		return 0, err
	}
	return maxVersion, nil
}

func writeTo(entry *pb.KV, w io.Writer) error {
	if err := binary.Write(w, binary.LittleEndian, uint64(entry.Size())); err != nil {
		return err
	}
	buf, err := entry.Marshal()
	if err != nil {
		return err
	}
	_, err = w.Write(buf)
	return err
}

// Load reads a protobuf-encoded list of all entries from a reader and writes
// them to the database. This can be used to restore the database from a backup
// made by calling DB.Backup().
//
// DB.Load() should be called on a database that is not running any other
// concurrent transactions while it is running.
func (db *DB) Load(r io.Reader) error {
	br := bufio.NewReaderSize(r, 16<<10)
	unmarshalBuf := make([]byte, 1<<10)
	var entries []*Entry
	var wg sync.WaitGroup
	errChan := make(chan error, 1)

	// func to check for pending error before sending off a batch for writing
	batchSetAsyncIfNoErr := func(entries []*Entry) error {
		select {
		case err := <-errChan:
			return err
		default:
			wg.Add(1)
			return db.batchSetAsync(entries, func(err error) {
				defer wg.Done()
				if err != nil {
					select {
					case errChan <- err:
					default:
					}
				}
			})
		}
	}

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

		e := &pb.KV{}
		if _, err = io.ReadFull(br, unmarshalBuf[:sz]); err != nil {
			return err
		}
		if err = e.Unmarshal(unmarshalBuf[:sz]); err != nil {
			return err
		}
		var userMeta byte
		if len(e.UserMeta) > 0 {
			userMeta = e.UserMeta[0]
		}
		entries = append(entries, &Entry{
			Key:       y.KeyWithTs(e.Key, e.Version),
			Value:     e.Value,
			UserMeta:  userMeta,
			ExpiresAt: e.ExpiresAt,
			meta:      e.Meta[0],
		})
		// Update nextTxnTs, memtable stores this timestamp in badger head
		// when flushed.
		if e.Version >= db.orc.nextTxnTs {
			db.orc.nextTxnTs = e.Version + 1
		}

		if len(entries) == 1000 {
			if err := batchSetAsyncIfNoErr(entries); err != nil {
				return err
			}
			entries = make([]*Entry, 0, 1000)
		}
	}

	if len(entries) > 0 {
		if err := batchSetAsyncIfNoErr(entries); err != nil {
			return err
		}
	}
	wg.Wait()

	select {
	case err := <-errChan:
		return err
	default:
		// Mark all versions done up until nextTxnTs.
		db.orc.txnMark.Done(db.orc.nextTxnTs - 1)
		return nil
	}
}
