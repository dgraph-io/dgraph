/*
 * Copyright 2017 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/dgraph-io/dgraph/x"
)

type waterMark struct {
	last uint64 // Last line number that was written to Badger.
	mark *x.WaterMark
	wg   *sync.WaitGroup
}

// A watermark for each file.
type syncMarks map[string]waterMark

// Create syncmarks for files and store them in dgraphClient.
func (l *loader) newSyncMarks(files []string) error {
	if len(files) == 0 {
		return nil
	}
	if len(l.marks) > 0 {
		return fmt.Errorf("NewSyncMarks should only be called once.")
	}

	for _, file := range files {
		ap, err := filepath.Abs(file)
		if err != nil {
			return err
		}

		if _, ok := l.marks[ap]; ok {
			return fmt.Errorf("Found duplicate file: %+v\n", ap)
		}
		l.marks.create(ap)
	}

	t := time.NewTicker(10 * time.Second)
	l.checkpointTicker = t
	go func(t *time.Ticker) {
		for range t.C {
			//l.writeCheckpoint()
		}
	}(t)
	return nil
}

func checkpointKey(file string) []byte {
	// Prefix to avoid key clashes with other data stored in badger.
	return []byte("\x02" + file)
}

/*
// Get checkpoint for file from Badger.
func (l *loader) checkpoint(file string) (uint64, error) {
	var item badger.KVItem
	err := l.kv.Get(checkpointKey(file), &item)
	if err != nil {
		return 0, err
	}
	var lineNum uint64
	err = item.Value(func(p []byte) error {
		if len(p) == 0 {
			// no existing checkpoint, so read from line 0
			return nil
		}
		var n int
		lineNum, n = binary.Uvarint(p)
		if n <= 0 {
			return x.Errorf("could not decode checkpoint (uvarint, n=%d)", n)
		}
		return nil
	})
	return lineNum, err
}


// Used to write checkpoints to Badger.
func (l *loader) writeCheckpoint() {
	wb := make([]*badger.Entry, 0, len(l.marks))
	for file, wm := range l.marks {
		doneUntil := wm.mark.DoneUntil()
		if doneUntil == 0 || doneUntil == wm.last {
			continue
		}
		wm.last = doneUntil
		l.marks[file] = wm
		var buf [binary.MaxVarintLen64]byte
		n := binary.PutUvarint(buf[:], doneUntil)
		wb = badger.EntriesSet(wb, checkpointKey(file), buf[:n])
	}

	if err := l.kv.BatchSet(wb); err != nil {
		fmt.Printf("Error while writing to disk %v\n", err)
	}
	for _, wbe := range wb {
		if err := wbe.Error; err != nil {
			fmt.Printf("Error while writing to disk %v\n", err)
		}
	}
}
*/
func (g syncMarks) create(file string) waterMark {
	x.AssertTrue(g != nil)

	if prev, present := g[file]; present {
		return prev
	}
	m := &x.WaterMark{Name: file}
	m.Init()
	wm := waterMark{mark: m, wg: new(sync.WaitGroup)}
	g[file] = wm
	return wm
}
