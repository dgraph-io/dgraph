/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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

package posting

import (
	"io/ioutil"
	"math"
	"os"
	"sync"
	"testing"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	bpb "github.com/dgraph-io/badger/v2/pb"
	"github.com/stretchr/testify/require"
)

func BenchmarkWriter(b *testing.B) {
	createKVList := func() bpb.KVList {
		var KVList bpb.KVList
		for i := 0; i < 50000; i++ {
			n := &bpb.KV{Key: []byte(string(i)), Value: []byte(string(i)), Version: 5}
			KVList.Kv = append(KVList.Kv, n)
		}
		return KVList
	}

	// Creates separate writer for each thread
	writeInBadgerMThreadsB := func(db *badger.DB, KVList *bpb.KVList, wg *sync.WaitGroup) {
		defer wg.Done()
		wb := db.NewManagedWriteBatch()
		if err := wb.Write(KVList); err != nil {
			panic(err)
		}
		require.NoError(b, wb.Flush())

	}

	// Resuses one writer for all threads
	writeInBadgerMThreadsW := func(wb *badger.WriteBatch, KVList *bpb.KVList, wg *sync.WaitGroup) {
		defer wg.Done()

		if err := wb.Write(KVList); err != nil {
			panic(err)
		}

	}
	// Creates separate writer for each thread
	writeInBadgerSingleThreadB := func(db *badger.DB, KVList *bpb.KVList) {
		wb := db.NewManagedWriteBatch()
		if err := wb.Write(KVList); err != nil {
			panic(err)
		}

	}
	// Resuses one writer for all threads
	writeInBadgerSingleThreadW := func(wb *badger.WriteBatch, KVList *bpb.KVList) {
		if err := wb.Write(KVList); err != nil {
			panic(err)
		}

	}

	dbOpts := badger.DefaultOptions("").
		WithLogger(nil).
		WithSyncWrites(false).
		WithNumVersionsToKeep(math.MaxInt64).
		WithCompression(options.None)

	KVList := createKVList()

	//Vanilla TxnWriter
	b.Run("TxnWriter", func(b *testing.B) {
		tmpIndexDir, err := ioutil.TempDir("", "dgraph")
		require.NoError(b, err)
		defer os.RemoveAll(tmpIndexDir)

		dbOpts.Dir = tmpIndexDir
		dbOpts.ValueDir = tmpIndexDir
		var db, err2 = badger.OpenManaged(dbOpts)
		require.NoError(b, err2)
		defer db.Close()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := NewTxnWriter(db)
			for _, typ := range KVList.Kv {
				k := typ.Key
				v := typ.Value
				err := w.SetAt(k, v, BitSchemaPosting, 1)
				require.NoError(b, err)
			}
			require.NoError(b, w.Flush())

		}
	})
	//Single threaded BatchWriter
	b.Run("WriteBatch1", func(b *testing.B) {
		tmpIndexDir, err := ioutil.TempDir("", "dgraph")
		require.NoError(b, err)
		defer os.RemoveAll(tmpIndexDir)

		dbOpts.Dir = tmpIndexDir
		dbOpts.ValueDir = tmpIndexDir

		var db, err2 = badger.OpenManaged(dbOpts)
		require.NoError(b, err2)
		defer db.Close()

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			wb := db.NewManagedWriteBatch()
			if err := wb.Write(&KVList); err != nil {
				panic(err)
			}
			require.NoError(b, wb.Flush())
		}
	})
	//Multi threaded Batchwriter with thread contention in WriteBatch
	b.Run("WriteBatchMultThreadB", func(b *testing.B) {
		tmpIndexDir, err := ioutil.TempDir("", "dgraph")
		require.NoError(b, err)
		defer os.RemoveAll(tmpIndexDir)

		dbOpts.Dir = tmpIndexDir
		dbOpts.ValueDir = tmpIndexDir

		var db, err2 = badger.OpenManaged(dbOpts)
		require.NoError(b, err2)
		defer db.Close()

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			var wg sync.WaitGroup
			wg.Add(5)

			go writeInBadgerMThreadsB(db, &bpb.KVList{Kv: KVList.Kv[:10000]}, &wg)
			go writeInBadgerMThreadsB(db, &bpb.KVList{Kv: KVList.Kv[10001:20000]}, &wg)
			go writeInBadgerMThreadsB(db, &bpb.KVList{Kv: KVList.Kv[20001:30000]}, &wg)
			go writeInBadgerMThreadsB(db, &bpb.KVList{Kv: KVList.Kv[30001:40000]}, &wg)
			go writeInBadgerMThreadsB(db, &bpb.KVList{Kv: KVList.Kv[40001:]}, &wg)
			wg.Wait()

		}
	})
	//Multi threaded Batchwriter with thread contention in SetEntry
	b.Run("WriteBatchMultThreadW", func(b *testing.B) {
		tmpIndexDir, err := ioutil.TempDir("", "dgraph")
		require.NoError(b, err)
		defer os.RemoveAll(tmpIndexDir)

		dbOpts.Dir = tmpIndexDir
		dbOpts.ValueDir = tmpIndexDir

		var db, err2 = badger.OpenManaged(dbOpts)
		require.NoError(b, err2)
		defer db.Close()

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			var wg sync.WaitGroup
			wg.Add(5)
			wb := db.NewManagedWriteBatch()
			go writeInBadgerMThreadsW(wb, &bpb.KVList{Kv: KVList.Kv[:10000]}, &wg)
			go writeInBadgerMThreadsW(wb, &bpb.KVList{Kv: KVList.Kv[10001:20000]}, &wg)
			go writeInBadgerMThreadsW(wb, &bpb.KVList{Kv: KVList.Kv[20001:30000]}, &wg)
			go writeInBadgerMThreadsW(wb, &bpb.KVList{Kv: KVList.Kv[30001:40000]}, &wg)
			go writeInBadgerMThreadsW(wb, &bpb.KVList{Kv: KVList.Kv[40001:]}, &wg)

			wg.Wait()
			require.NoError(b, wb.Flush())
		}
	})
	b.Run("WriteBatchSingleThreadB", func(b *testing.B) {
		tmpIndexDir, err := ioutil.TempDir("", "dgraph")
		require.NoError(b, err)
		defer os.RemoveAll(tmpIndexDir)

		dbOpts.Dir = tmpIndexDir
		dbOpts.ValueDir = tmpIndexDir

		var db, err2 = badger.OpenManaged(dbOpts)
		require.NoError(b, err2)
		defer db.Close()

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			var wg sync.WaitGroup
			wg.Add(5)
			wb := db.NewManagedWriteBatch()
			writeInBadgerSingleThreadB(db, &bpb.KVList{Kv: KVList.Kv[:10000]})
			writeInBadgerSingleThreadB(db, &bpb.KVList{Kv: KVList.Kv[10001:20000]})
			writeInBadgerSingleThreadB(db, &bpb.KVList{Kv: KVList.Kv[20001:30000]})
			writeInBadgerSingleThreadB(db, &bpb.KVList{Kv: KVList.Kv[30001:40000]})
			writeInBadgerSingleThreadB(db, &bpb.KVList{Kv: KVList.Kv[40001:]})

			wg.Wait()
			require.NoError(b, wb.Flush())
		}
	})
	b.Run("WriteBatchSingleThreadW", func(b *testing.B) {
		tmpIndexDir, err := ioutil.TempDir("", "dgraph")
		require.NoError(b, err)
		defer os.RemoveAll(tmpIndexDir)

		dbOpts.Dir = tmpIndexDir
		dbOpts.ValueDir = tmpIndexDir

		var db, err2 = badger.OpenManaged(dbOpts)
		require.NoError(b, err2)
		defer db.Close()

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			var wg sync.WaitGroup
			wg.Add(5)
			wb := db.NewManagedWriteBatch()
			writeInBadgerSingleThreadW(wb, &bpb.KVList{Kv: KVList.Kv[:10000]})
			writeInBadgerSingleThreadW(wb, &bpb.KVList{Kv: KVList.Kv[10001:20000]})
			writeInBadgerSingleThreadW(wb, &bpb.KVList{Kv: KVList.Kv[20001:30000]})
			writeInBadgerSingleThreadW(wb, &bpb.KVList{Kv: KVList.Kv[30001:40000]})
			writeInBadgerSingleThreadW(wb, &bpb.KVList{Kv: KVList.Kv[40001:]})

			wg.Wait()
			require.NoError(b, wb.Flush())
		}
	})
}
