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

package client

import (
	"encoding/binary"
	"fmt"
	"path/filepath"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/x"
)

type waterMark struct {
	last uint64 // Last line number that was written to Badger.
	mark *x.WaterMark
}

// A watermark for each file.
type syncMarks map[string]waterMark

// Create syncmarks for files and store them in dgraphClient.
func (d *Dgraph) NewSyncMarks(files []string) error {
	if d.marks != nil {
		return fmt.Errof("NewSyncMarks should only be called once.")
	}

	for _, file := range files {
		ap, err := filepath.Abs(file)
		if err != nil {
			return err
		}

		if _, ok := d.marks[ap]; ok {
			return fmt.Errorf("Found duplicate file: %+v\n", ap)
		}
		d.marks.create(ap)
	}

	t := time.NewTicker(time.Minute)
	d.checkpointTicker = t
	go d.storeCheckpoint(t)
	return nil
}

// Get checkpoint for file from Badger.
func (d *Dgraph) Checkpoint(file string) (uint64, error) {
	return d.alloc.getFromKV(fmt.Sprintf("checkpoint-%s", file))
}

// After we have received response from server and sent the marks for completion,
// we need to wait for all of them to be processed.
func (d *Dgraph) syncAllMarks() {
	for _, wm := range d.marks {
		for wm.mark.WaitingFor() {
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// Used to write checkpoints to Badger.
func (d *Dgraph) writeCheckpoint() {
	wb := make([]*badger.Entry, 0, len(d.marks))
	for file, wm := range d.marks {
		doneUntil := wm.mark.DoneUntil()
		if doneUntil == 0 || doneUntil == wm.last {
			continue
		}
		wm.last = doneUntil
		d.marks[file] = wm
		var buf [10]byte
		n := binary.PutUvarint(buf[:], doneUntil)
		wb = badger.EntriesSet(wb, []byte(fmt.Sprintf("checkpoint-%s", file)), buf[:n])
	}

	if err := d.alloc.kv.BatchSet(wb); err != nil {
		fmt.Printf("Error while writing to disk %v\n", err)
	}
	for _, wbe := range wb {
		if err := wbe.Error; err != nil {
			fmt.Printf("Error while writing to disk %v\n", err)
		}
	}
}

// Used to store checkpoints for various files periodically.
func (d *Dgraph) storeCheckpoint(ticker *time.Ticker) {
	for range ticker.C {
		d.writeCheckpoint()
	}
}

func (g syncMarks) create(file string) waterMark {
	if g == nil {
		g = make(map[string]waterMark)
	}

	if prev, present := g[file]; present {
		return prev
	}
	m := &x.WaterMark{Name: file}
	m.Init()
	wm := waterMark{mark: m}
	g[file] = wm
	return wm
}
