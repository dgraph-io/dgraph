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

package schema

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/dgraph-io/badger"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

const (
	syncChCapacity = 10000
)

var (
	pstate *stateGroup
	pstore *badger.KV
	syncCh chan SyncEntry
)

type stateGroup struct {
	// Can have fine grained locking later if necessary, per group or predicate
	sync.RWMutex // x.SafeMutex is slow.
	// Map containing predicate to type information.
	predicate map[string]*protos.SchemaUpdate
	elog      trace.EventLog
}

func (s *stateGroup) init() {
	s.predicate = make(map[string]*protos.SchemaUpdate)
	s.elog = trace.NewEventLog("Dgraph", "Schema")
}

type state struct {
	sync.RWMutex
	m    map[uint32]*stateGroup
	elog trace.EventLog
}

// SateFor returns the schema for given group
func State() *stateGroup {
	return pstate
}

func (s *stateGroup) DeleteAll() {
	s.Lock()
	defer s.Unlock()

	for pred := range s.predicate {
		// We set schema for _predicate_, hence it shouldn't be deleted.
		if pred != "_predicate_" {
			delete(s.predicate, pred)
		}
	}
}

// Update updates the schema in memory and sends an entry to syncCh so that it can be
// committed later
func (s *stateGroup) Update(se SyncEntry) {
	s.Lock()
	defer s.Unlock()

	s.predicate[se.Attr] = &se.Schema
	se.Water.Begin(se.Index)
	syncCh <- se
	s.elog.Printf(logUpdate(se.Schema, se.Attr))
	x.Printf(logUpdate(se.Schema, se.Attr))
}

// Delete updates the schema in memory and sends an entry to syncCh so that it can be
// committed later
func (s *stateGroup) Delete(se SyncEntry) {
	s.Lock()
	defer s.Unlock()

	delete(s.predicate, se.Attr)
	se.Water.Begin(se.Index)
	syncCh <- se
	s.elog.Printf("Deleting schema for attr: %s", se.Attr)
	x.Printf("Deleting schema for attr: %s", se.Attr)
}

// Remove deletes the schema from memory and disk. Used after predicate move to do
// cleanup
func (s *stateGroup) Remove(predicate string) error {
	s.Lock()
	defer s.Unlock()

	delete(s.predicate, predicate)
	return pstore.Delete(x.SchemaKey(predicate))
}

func logUpdate(schema protos.SchemaUpdate, pred string) string {
	typ := types.TypeID(schema.ValueType).Name()
	if schema.List {
		typ = fmt.Sprintf("[%s]", typ)
	}
	return fmt.Sprintf("Setting schema for attr %s: %v, tokenizer: %v, directive: %v, count: %v\n",
		pred, typ, schema.Tokenizer, schema.Directive, schema.Count)
}

// Set sets the schema for given predicate in memory
// schema mutations must flow through update function, which are
// synced to db
func (s *stateGroup) Set(pred string, schema protos.SchemaUpdate) {
	s.Lock()
	defer s.Unlock()
	s.predicate[pred] = &schema
	s.elog.Printf(logUpdate(schema, pred))
}

// Get gets the schema for given predicate
func (s *stateGroup) Get(pred string) (protos.SchemaUpdate, bool) {
	s.Lock()
	defer s.Unlock()
	schema, has := s.predicate[pred]
	if !has {
		return protos.SchemaUpdate{}, false
	}
	return *schema, true
}

// TypeOf returns the schema type of predicate
func (s *stateGroup) TypeOf(pred string) (types.TypeID, error) {
	s.RLock()
	defer s.RUnlock()
	if schema, ok := s.predicate[pred]; ok {
		return types.TypeID(schema.ValueType), nil
	}
	return types.TypeID(100), x.Errorf("Schema not defined for predicate: %v.", pred)
}

// IsIndexed returns whether the predicate is indexed or not
func (s *stateGroup) IsIndexed(pred string) bool {
	s.RLock()
	defer s.RUnlock()
	if schema, ok := s.predicate[pred]; ok {
		return len(schema.Tokenizer) > 0
	}
	return false
}

// IndexedFields returns the list of indexed fields
func (s *stateGroup) IndexedFields() []string {
	s.RLock()
	defer s.RUnlock()
	var out []string
	for k, v := range s.predicate {
		if len(v.Tokenizer) > 0 {
			out = append(out, k)
		}
	}
	return out
}

// Predicates returns the list of predicates for given group
func (s *stateGroup) Predicates() []string {
	s.RLock()
	defer s.RUnlock()
	out := make([]string, 0, len(s.predicate))
	for k := range s.predicate {
		out = append(out, k)
	}
	return out
}

// Tokenizer returns the tokenizer for given predicate
func (s *stateGroup) Tokenizer(pred string) []tok.Tokenizer {
	s.RLock()
	defer s.RUnlock()
	schema, ok := s.predicate[pred]
	x.AssertTruef(ok, "schema state not found for %s", pred)
	var tokenizers []tok.Tokenizer
	for _, it := range schema.Tokenizer {
		t, has := tok.GetTokenizer(it)
		x.AssertTruef(has, "Invalid tokenizer %s", it)
		tokenizers = append(tokenizers, t)
	}
	return tokenizers
}

// TokenizerNames returns the tokenizer names for given predicate
func (s *stateGroup) TokenizerNames(pred string) []string {
	s.RLock()
	defer s.RUnlock()
	schema, ok := s.predicate[pred]
	x.AssertTruef(ok, "schema state not found for %s", pred)
	var tokenizers []string
	for _, it := range schema.Tokenizer {
		t, found := tok.GetTokenizer(it)
		x.AssertTruef(found, "Tokenizer not found for %s", it)
		tokenizers = append(tokenizers, t.Name())
	}
	return tokenizers
}

// IsReversed returns whether the predicate has reverse edge or not
func (s *stateGroup) IsReversed(pred string) bool {
	s.RLock()
	defer s.RUnlock()
	if schema, ok := s.predicate[pred]; ok {
		return schema.Directive == protos.SchemaUpdate_REVERSE
	}
	return false
}

// HasCount returns whether we want to mantain a count index for the given predicate or not.
func (s *stateGroup) HasCount(pred string) bool {
	s.RLock()
	defer s.RUnlock()
	if schema, ok := s.predicate[pred]; ok {
		return schema.Count
	}
	return false
}

// IsList returns whether the predicate is of list type.
func (s *stateGroup) IsList(pred string) bool {
	s.RLock()
	defer s.RUnlock()
	if schema, ok := s.predicate[pred]; ok {
		return schema.List
	}
	return false
}

func Init(ps *badger.KV) {
	pstore = ps
	syncCh = make(chan SyncEntry, syncChCapacity)
	reset()
	go batchSync()
}

// LoadFromDb reads schema information from db and stores it in memory
func LoadFromDb() error {
	prefix := x.SchemaPrefix()
	itr := pstore.NewIterator(badger.DefaultIteratorOptions) // Need values, reversed=false.
	defer itr.Close()

	for itr.Seek(prefix); itr.Valid(); itr.Next() {
		item := itr.Item()
		key := item.Key()
		if !bytes.HasPrefix(key, prefix) {
			break
		}
		attr := x.Parse(key).Attr
		var s protos.SchemaUpdate
		err := item.Value(func(val []byte) error {
			x.Checkf(s.Unmarshal(val), "Error while loading schema from db")
			return nil
		})
		if err != nil {
			return err
		}
		State().Set(attr, s)
	}
	return nil
}

func reset() {
	pstate = new(stateGroup)
	pstate.init()
}

// SyncEntry stores the schema mutation information
type SyncEntry struct {
	Attr   string
	Schema protos.SchemaUpdate
	Water  *x.WaterMark
	Index  uint64
}

func addToEntriesMap(entriesMap map[*x.WaterMark][]uint64, entries []SyncEntry) {
	for _, entry := range entries {
		if entry.Water != nil {
			entriesMap[entry.Water] = append(entriesMap[entry.Water], entry.Index)
		}
	}
}

func batchSync() {
	var entries []SyncEntry
	var loop uint64
	wb := make([]*badger.Entry, 0, 100)
	for {
		ent := <-syncCh
	slurpLoop:
		for {
			entries = append(entries, ent)
			if len(entries) == syncChCapacity {
				// Avoid making infinite batch, push back against syncCh.
				break
			}
			select {
			case ent = <-syncCh:
			default:
				break slurpLoop
			}
		}

		loop++
		State().elog.Printf("[%4d] Writing schema batch of size: %v\n", loop, len(entries))
		for _, e := range entries {
			if e.Schema.Directive == protos.SchemaUpdate_DELETE {
				wb = badger.EntriesDelete(wb, x.SchemaKey(e.Attr))
				continue
			}
			val, err := e.Schema.Marshal()
			x.Checkf(err, "Error while marshalling schema description")
			wb = badger.EntriesSet(wb, x.SchemaKey(e.Attr), val)
		}
		pstore.BatchSet(wb)
		wb = wb[:0]

		entriesMap := make(map[*x.WaterMark][]uint64)
		addToEntriesMap(entriesMap, entries)
		for wm, indices := range entriesMap {
			wm.DoneMany(indices)
		}
		entries = entries[:0]
	}
}
