/*
 * Copyright 2016 Dgraph Labs, Inc.
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
	"sync"

	"github.com/dgraph-io/dgraph/x"
)

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
