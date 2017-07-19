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
	"sync"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/x"
)

func (d *Dgraph) Checkpoint(file string) (uint64, error) {
	return d.alloc.getFromKV(fmt.Sprintf("checkpoint-%s", file))
}

// Used to store checkpoints for various files.
func (d *Dgraph) storeCheckpoint() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	wb := make([]*badger.Entry, 0, 5)
	for range ticker.C {
		m := d.marks.watermarks()
		if m == nil {
			return
		}

		for file, w := range m {
			var buf []byte
			n := binary.PutUvarint(buf, w)
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
		wb = wb[:0]
	}
}

//func NewSyncMarks(files []string) {
//	for _, file := range files {
//		bp := filepath.Base(file)
//	}
//}

type syncMarks struct {
	sync.RWMutex
	// A watermark for each file.
	m map[string]*x.WaterMark
}

func (g *syncMarks) create(file string) *x.WaterMark {
	g.Lock()
	defer g.Unlock()
	if g.m == nil {
		g.m = make(map[string]*x.WaterMark)
	}

	if prev, present := g.m[file]; present {
		return prev
	}
	w := &x.WaterMark{Name: file}
	w.Init()
	g.m[file] = w
	return w
}

func (g *syncMarks) Get(file string) *x.WaterMark {
	g.RLock()
	if w, present := g.m[file]; present {
		g.RUnlock()
		return w
	}
	g.RUnlock()
	return g.create(file)
}

func (d *Dgraph) SyncMarkFor(file string) *x.WaterMark {
	return d.marks.Get(file)
}

func (g *syncMarks) watermarks() map[string]uint64 {
	g.RLock()
	defer g.RUnlock()

	if len(g.m) == 0 {
		return nil
	}

	m := make(map[string]uint64)
	for f, w := range g.m {
		m[f] = w.DoneUntil()
	}
	return m
}
