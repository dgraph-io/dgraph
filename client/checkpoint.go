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

// A watermark for each file
type syncMarks map[string]*x.WaterMark

// Create syncmarks for files and store them in dgraphClient.
func (d *Dgraph) NewSyncMarks(files []string) {
	for _, file := range files {
		bp := filepath.Base(file)
		d.marks.create(bp)
	}
}

// Get checkpoint for file from Badger.
func (d *Dgraph) Checkpoint(file string) (uint64, error) {
	return d.alloc.getFromKV(fmt.Sprintf("checkpoint-%s", file))
}

// This is called to syncAllMarks. It is useful to find the final line that was processed for each
// file.
func (d *Dgraph) syncAllMarks() {
	for _, mark := range d.marks {
		for mark.WaitingFor() {
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// Used to write checkpoints to Badger.
func (d *Dgraph) writeCheckpoint() {
	wb := make([]*badger.Entry, 0, len(d.marks))
	for file, mark := range d.marks {
		w := mark.DoneUntil()
		if w == 0 {
			continue
		}
		var buf [10]byte
		n := binary.PutUvarint(buf[:], w)
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
func (d *Dgraph) storeCheckpoint() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		d.writeCheckpoint()
	}
}

func (g syncMarks) create(file string) *x.WaterMark {
	if g == nil {
		g = make(map[string]*x.WaterMark)
	}

	if prev, present := g[file]; present {
		return prev
	}
	w := &x.WaterMark{Name: file}
	w.Init()
	g[file] = w
	return w
}
