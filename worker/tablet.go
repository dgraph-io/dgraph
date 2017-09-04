/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package worker

import (
	"bytes"
	"sync"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/x"
)

type KeyRange struct {
	Predicate string
	UidStart  uint64 // inclusive.
	UidEnd    uint64 // inclusive. If zero, then consider infinity.
}

type Tablet struct {
	size uint64
	kr   KeyRange
}

// EstimateSize for now only estimates the data size, not the index size.
func (t *Tablet) EstimateSize() (total int64) {
	opt := badger.DefaultIteratorOptions
	opt.FetchValues = false
	key := x.DataKey(t.kr.Predicate, t.kr.UidStart)
	endKey := x.DataKey(t.kr.Predicate, t.kr.UidEnd)
	itr := pstore.NewIterator(opt)
	for itr.Seek(key); itr.Valid(); itr.Next() {
		item := itr.Item()
		if bytes.Compare(item.Key(), endKey) > 0 {
			break
		}
		total += item.EstimateSize()
	}
	itr.Close()
	return total
}

type Tablets struct {
	sync.RWMutex
	tmap map[string]*tablet
}

func (t *Tablets) Get(pred string) *Tablet {
	t.RLock()
	defer t.RUnlock()
	if tab, ok := t.tmap[pred]; ok {
		return tab
	}
	return nil
}

func (t *Tablets) AddIfAbsent(pred string, start, end uint64) {
	if tab := t.BelongsTo(pred); tab != nil {
		return
	}

	t.Lock()
	defer t.Unlock()
	if tab, ok := t.tmap[pred]; ok {
		return
	}
	kr := KeyRange{Predicate: pred, UidStart: start, UidEnd: end}
	tab := &Tablet{krange: kr}
	t.tmap[pred] = tab
}
