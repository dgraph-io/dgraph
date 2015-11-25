/*
 * Copyright 2015 Manish R Jain <manishrjain@gmail.com>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package commit

import (
	"bytes"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestHandleFile(t *testing.T) {
	dir, err := ioutil.TempDir("", "dgraph-log")
	if err != nil {
		t.Error(err)
		return
	}
	defer os.RemoveAll(dir)

	l := NewLogger(dir, "dgraph", 50<<20)
	ts := time.Now().UnixNano()
	for i := 0; i < 10; i++ {
		fp := filepath.Join(dir, l.filepath(ts+int64(i)))
		if err := ioutil.WriteFile(fp, []byte("test calling"),
			os.ModeAppend); err != nil {
			t.Error(err)
			return
		}
	}
	l.Init()
	for i, lf := range l.list {
		exp := ts + int64(i)
		if lf.endTs != exp {
			t.Errorf("Expected %v. Got: %v", exp, lf.endTs)
		}
	}
}

func TestAddLog(t *testing.T) {
	dir, err := ioutil.TempDir("", "dgraph-log")
	if err != nil {
		t.Error(err)
		return
	}
	defer os.RemoveAll(dir)

	l := NewLogger(dir, "dgraph", 50<<20)
	l.SyncDur = time.Millisecond
	l.SyncEvery = 1000 // So, sync after write never gets called.
	l.Init()
	defer l.Close()

	ts := time.Now().UnixNano()
	for i := 0; i < 10; i++ {
		curts := ts + int64(i)
		if err := l.AddLog(curts, 0, []byte("hey")); err != nil {
			t.Error(err)
			return
		}
		time.Sleep(500 * time.Microsecond)
	}

	glog.Debugf("Test curfile path: %v", l.cf.f.Name())
	last, err := lastTimestamp(l.cf.cache())
	if err != nil {
		t.Error(err)
	}
	if last != ts+9 {
		t.Errorf("Expected %v. Got: %v\n", ts+9, last)
	}
}

func TestRotatingLog(t *testing.T) {
	dir, err := ioutil.TempDir("", "dgraph-log")
	if err != nil {
		t.Error(err)
		return
	}
	defer os.RemoveAll(dir)

	l := NewLogger(dir, "dgraph", 1024) // 1 kB
	l.SyncDur = 0
	l.SyncEvery = 0
	l.Init()

	data := make([]byte, 400)
	ts := time.Now().UnixNano()
	for i := 0; i < 9; i++ {
		curts := ts + int64(i)
		if err := l.AddLog(curts, 0, data); err != nil {
			t.Error(err)
			return
		}
	}
	// This should have created 4 files of 832 bytes each (header + data), and
	// the current file should be of size 416.
	if len(l.list) != 4 {
		t.Errorf("Expected 4 files. Got: %v", len(l.list))
	}
	for i, lf := range l.list {
		exp := ts + int64(2*i+1)
		if lf.endTs != exp {
			t.Errorf("Expected end ts: %v. Got: %v", exp, lf.endTs)
		}
	}
	if l.curFile().Size() != 416 {
		t.Errorf("Expected size 416. Got: %v", l.curFile().Size())
	}
	if atomic.LoadInt64(&l.lastLogTs) != ts+int64(8) {
		t.Errorf("Expected ts: %v. Got: %v", ts+int64(8), l.lastLogTs)
	}
	l.Close()
	l = nil // Important to avoid re-use later.

	// Now, let's test a re-init of logger.
	nl := NewLogger(dir, "dgraph", 1024)
	nl.Init()
	defer nl.Close()
	if len(nl.list) != 4 {
		t.Errorf("Expected 4 files. Got: %v", len(nl.list))
	}
	if nl.curFile().Size() != 416 {
		t.Errorf("Expected size 416. Got: %v", nl.curFile().Size())
	}
	if err := nl.AddLog(ts, 0, data); err == nil {
		t.Error("Adding an entry with older ts should fail.")
	}
	if err := nl.AddLog(ts+int64(100), 0, data); err != nil {
		t.Error(err)
		return
	}
	if nl.curFile().Size() != 832 {
		t.Errorf("Expected size 832. Got: %v", nl.curFile().Size())
	}
	if err := nl.AddLog(ts+int64(113), 0, data); err != nil {
		t.Error(err)
		return
	}
	if len(nl.list) != 5 {
		t.Errorf("Expected 5 files. Got: %v", len(nl.list))
	}
	if nl.list[4].endTs != ts+int64(100) {
		t.Errorf("Expected ts: %v. Got: %v", ts+int64(100), nl.list[4].endTs)
	}
	if nl.curFile().Size() != 416 {
		t.Errorf("Expected size 416. Got: %v", nl.curFile().Size())
	}
	if nl.lastLogTs != ts+int64(113) {
		t.Errorf("Expected last log ts: %v. Got: %v", ts+int64(113), nl.lastLogTs)
	}
}

func TestReadEntries(t *testing.T) {
	dir, err := ioutil.TempDir("", "dgraph-log")
	if err != nil {
		t.Error(err)
		return
	}
	defer os.RemoveAll(dir)

	l := NewLogger(dir, "dgraph", 1024) // 1 kB
	l.SyncDur = 0
	l.SyncEvery = 0
	l.Init()
	defer l.Close()

	data := make([]byte, 400)
	ts := time.Now().UnixNano()
	for i := 0; i < 9; i++ {
		curts := ts + int64(i)
		if err := l.AddLog(curts, uint32(i%3), data); err != nil {
			t.Error(err)
			return
		}
	}
	// This should have created 4 files of 832 bytes each (header + data), and
	// the current file should be of size 416.
	if len(l.list) != 4 {
		t.Errorf("Expected 4 files. Got: %v", len(l.list))
	}
	for i, lf := range l.list {
		exp := ts + int64(2*i+1)
		if lf.endTs != exp {
			t.Errorf("Expected end ts: %v. Got: %v", exp, lf.endTs)
		}
	}
	if l.curFile().Size() != 416 {
		t.Errorf("Expected size 416. Got: %v", l.curFile().Size())
	}
	if l.lastLogTs != ts+int64(8) {
		t.Errorf("Expected ts: %v. Got: %v", ts+int64(8), l.lastLogTs)
	}

	{
		// Check for hash = 1, ts >= 2.
		count := 0
		err := l.StreamEntries(ts+2, uint32(1), func(hdr Header, entry []byte) {
			count += 1
			if bytes.Compare(data, entry) != 0 {
				t.Error("Data doesn't equate.")
			}
		})
		if err != nil {
			t.Error(err)
		}
		if count != 2 {
			t.Errorf("Expected 2 entries. Got: %v", count)
		}
	}
	{
		if err := l.AddLog(ts+int64(9), 1, data); err != nil {
			t.Error(err)
		}
		// Check for hash = 1, ts >= 2.
		count := 0
		err := l.StreamEntries(ts, uint32(1), func(hdr Header, entry []byte) {
			count += 1
			if bytes.Compare(data, entry) != 0 {
				t.Error("Data doesn't equate.")
			}
		})
		if err != nil {
			t.Error(err)
		}
		if count != 4 {
			t.Errorf("Expected 4 entries. Got: %v", count)
		}
	}
}

func benchmarkAddLog(n int, b *testing.B) {
	dir, err := ioutil.TempDir("", "dgraph-log")
	if err != nil {
		b.Error(err)
		return
	}
	defer os.RemoveAll(dir)

	l := NewLogger(dir, "dgraph", 50<<20)
	l.SyncEvery = n
	l.Init()

	data := make([]byte, 100)
	b.ResetTimer()
	ts := time.Now().UnixNano()
	for i := 0; i < b.N; i++ {
		end := rand.Intn(50)
		if err := l.AddLog(ts+int64(i), 0, data[:50+end]); err != nil {
			b.Error(err)
		}
	}
	l.Close()
}

func BenchmarkAddLog_SyncEveryRecord(b *testing.B)      { benchmarkAddLog(0, b) }
func BenchmarkAddLog_SyncEvery10Records(b *testing.B)   { benchmarkAddLog(10, b) }
func BenchmarkAddLog_SyncEvery100Records(b *testing.B)  { benchmarkAddLog(100, b) }
func BenchmarkAddLog_SyncEvery1000Records(b *testing.B) { benchmarkAddLog(1000, b) }
