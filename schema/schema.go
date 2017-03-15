/*
 * Copyright 2016 DGraph Labs, Inc.
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

package schema

import (
	"fmt"
	"time"

	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/protos/typesp"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

var (
	pstate *state
	pstore *store.Store
	syncCh chan *SyncEntry
)

type stateGroup struct {
	// Can have fine grained locking later if necessary, per group or predicate
	x.SafeMutex
	// Map containing predicate to type information.
	predicate map[string]*typesp.Schema
	elog      trace.EventLog
}

func (s *stateGroup) init(group uint32) {
	s.predicate = make(map[string]*typesp.Schema)
	s.elog = trace.NewEventLog("Dynamic Schema", fmt.Sprintf("%d", group))
}

type state struct {
	x.SafeMutex
	m    map[uint32]*stateGroup
	elog trace.EventLog
}

func (s *state) create(group uint32) *stateGroup {
	s.Lock()
	defer s.Unlock()
	if s.m == nil {
		s.m = make(map[uint32]*stateGroup)
	}

	if prev, present := s.m[group]; present {
		return prev
	}
	shard := &stateGroup{}
	shard.init(group)
	s.m[group] = shard
	return shard
}

func (s *state) get(group uint32) *stateGroup {
	s.RLock()
	if shard, present := s.m[group]; present {
		s.RUnlock()
		return shard
	}
	s.RUnlock()
	return s.create(group)
}

// SateFor returns the schema for given group
func State() *state {
	return pstate
}

// Update updates the schema in memory and sends an entry to syncCh so that it can be
// comitted later
func (s *state) Update(se *SyncEntry) {
	s.get(group.BelongsTo(se.Attr)).update(se)
}

func (s *stateGroup) update(se *SyncEntry) {
	s.Lock()
	defer s.Unlock()

	_, ok := s.predicate[se.Attr]
	x.AssertTruef(!ok, "Schema doesn't exist for attribute %s", se.Attr)

	// Creating a copy to avoid race condition during marshalling
	schema := se.Schema
	s.predicate[se.Attr] = &schema
	se.Water.Ch <- x.Mark{Index: se.Index, Done: false}
	syncCh <- se
	s.elog.Printf("Setting schema for attr %s: %v\n", se.Attr, se.Schema.ValueType)
}

// SetType sets the schema type for given predicate
func (s *state) SetType(pred string, valueType types.TypeID) {
	s.get(group.BelongsTo(pred)).setType(pred, valueType)
}

func (s *stateGroup) setType(pred string, valueType types.TypeID) {
	s.Lock()
	defer s.Unlock()
	if schema, ok := s.predicate[pred]; ok {
		schema.ValueType = uint32(valueType)
	} else {
		s.predicate[pred] = &typesp.Schema{ValueType: uint32(valueType)}
	}
}

// SetReverse sets whether the reverse edge is enabled or
// not for given predicate, if schema is not already defined, it's set to uid type
func (s *state) SetReverse(pred string, rev bool) {
	s.get(group.BelongsTo(pred)).setReverse(pred, rev)
}

func (s *stateGroup) setReverse(pred string, rev bool) {
	s.Lock()
	defer s.Unlock()
	if schema, ok := s.predicate[pred]; !ok {
		s.predicate[pred] = &typesp.Schema{ValueType: uint32(types.UidID), Reverse: rev}
	} else {
		x.AssertTruef(schema.ValueType == uint32(types.UidID),
			"predicate %s is not of type uid", pred)
		schema.Reverse = rev
	}
}

// AddIndex adds the tokenizer for given predicate
func (s *state) AddIndex(pred string, tokenizer string) {
	s.get(group.BelongsTo(pred)).addIndex(pred, tokenizer)
}

func (s *stateGroup) addIndex(pred string, tokenizer string) {
	s.Lock()
	defer s.Unlock()
	schema, ok := s.predicate[pred]
	x.AssertTruef(ok, "schema state not found for %s", pred)
	// Check for duplicates.
	for _, tok := range schema.Tokenizer {
		if tok == tokenizer {
			return
		}
	}
	schema.Tokenizer = append(schema.Tokenizer, tokenizer)
}

// Set sets the schema for given predicate
func (s *state) Set(pred string, schema *typesp.Schema) {
	s.get(group.BelongsTo(pred)).set(pred, schema)
}

func (s *stateGroup) set(pred string, schema *typesp.Schema) {
	s.Lock()
	defer s.Unlock()
	s.predicate[pred] = schema
	s.elog.Printf("Setting schema for attr %s: %v\n", pred, schema.ValueType)
}

// TypeOf returns the schema type of predicate
func (s *state) TypeOf(pred string) (types.TypeID, error) {
	return s.get(group.BelongsTo(pred)).typeOf(pred)
}

func (s *stateGroup) typeOf(pred string) (types.TypeID, error) {
	s.RLock()
	defer s.RUnlock()
	if schema, ok := s.predicate[pred]; ok {
		return types.TypeID(schema.ValueType), nil
	}
	return types.TypeID(100), x.Errorf("Undefined predicate")
}

// IsIndexed returns whether the predicate is indexed or not
func (s *state) IsIndexed(pred string) bool {
	return s.get(group.BelongsTo(pred)).isIndexed(pred)
}

func (s *stateGroup) isIndexed(pred string) bool {
	s.RLock()
	defer s.RUnlock()
	if schema, ok := s.predicate[pred]; ok {
		return len(schema.Tokenizer) > 0
	}
	return false
}

// IndexedFields returns the list of indexed fields
func (s *state) IndexedFields(gid uint32) []string {
	return s.get(gid).indexedFields()
}

func (s *stateGroup) indexedFields() []string {
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
func (s *state) Predicates(group uint32) []string {
	return s.get(group).predicates()
}

func (s *stateGroup) predicates() []string {
	s.RLock()
	defer s.RUnlock()
	out := make([]string, 0, len(s.predicate))
	for k := range s.predicate {
		out = append(out, k)
	}
	return out
}

// Tokenizer returns the tokenizer for given predicate
func (s *state) Tokenizer(pred string) []tok.Tokenizer {
	return s.get(group.BelongsTo(pred)).tokenizer(pred)
}

func (s *stateGroup) tokenizer(pred string) []tok.Tokenizer {
	s.RLock()
	defer s.RUnlock()
	schema, ok := s.predicate[pred]
	x.AssertTruef(ok, "schema state not found for %s", pred)
	var tokenizers []tok.Tokenizer
	for _, it := range schema.Tokenizer {
		tokenizers = append(tokenizers, tok.GetTokenizer(it))
	}
	return tokenizers
}

// Tokenizer returns the tokenizer names for given predicate
func (s *state) TokenizerNames(pred string) []string {
	return s.get(group.BelongsTo(pred)).tokenizerNames(pred)
}

func (s *stateGroup) tokenizerNames(pred string) []string {
	s.RLock()
	defer s.RUnlock()
	schema, ok := s.predicate[pred]
	x.AssertTruef(ok, "schema state not found for %s", pred)
	var tokenizers []string
	for _, it := range schema.Tokenizer {
		tokenizers = append(tokenizers, tok.GetTokenizer(it).Name())
	}
	return tokenizers
}

// IsReversed returns whether the predicate has reverse edge or not
func (s *state) IsReversed(pred string) bool {
	return s.get(group.BelongsTo(pred)).isReversed(pred)
}

func (s *stateGroup) isReversed(pred string) bool {
	s.RLock()
	defer s.RUnlock()
	if schema, ok := s.predicate[pred]; ok {
		return schema.Reverse
	}
	return false
}

func Init(ps *store.Store) {
	pstore = ps
	syncCh = make(chan *SyncEntry, 10000)
	go batchSync()
}

// ReloadData loads schema from file and then later from db
func ReloadData(file string, group uint32) error {
	reset()
	if len(file) > 0 {
		if err := parse(file, group); err != nil {
			return err
		}
	}
	if err := LoadFromDb(group); err != nil {
		return err
	}
	return nil
}

// LoadFromDb reads schema information from db and stores it in memory
// This is used on server start to load schema for all groups, avoid repeated
// query to disk if we have large number of groups
func LoadFromDb(gid uint32) error {
	prefix := x.SchemaPrefix()
	itr := pstore.NewIterator()
	defer itr.Close()

	for itr.Seek(prefix); itr.ValidForPrefix(prefix); itr.Next() {
		key := itr.Key().Data()
		attr := x.Parse(key).Attr
		data := itr.Value().Data()
		var s typesp.Schema
		x.Checkf(s.Unmarshal(data), "Error while loading schema from db")
		if group.BelongsTo(attr) != gid {
			continue
		}
		State().Set(attr, &s)
	}
	return nil
}

func Refresh(groupId uint32) error {
	prefix := x.SchemaPrefix()
	itr := pstore.NewIterator()
	defer itr.Close()

	for itr.Seek(prefix); itr.ValidForPrefix(prefix); itr.Next() {
		key := itr.Key().Data()
		attr := x.Parse(key).Attr
		if group.BelongsTo(attr) != groupId {
			continue
		}
		data := itr.Value().Data()
		var s typesp.Schema
		x.Checkf(s.Unmarshal(data), "Error while loading schema from db")
		State().Set(attr, &s)
	}

	return nil
}

func reset() {
	pstate = new(state)
	pstate.elog = trace.NewEventLog("Dynamic Schema", "schema")
}

// SyncEntry stores the schema mutation information
type SyncEntry struct {
	Attr   string
	Schema typesp.Schema
	Water  *x.WaterMark
	Index  uint64
}

func addToEntriesMap(entriesMap map[chan x.Mark][]uint64, entries []*SyncEntry) {
	for _, entry := range entries {
		if entry.Water != nil {
			entriesMap[entry.Water.Ch] = append(entriesMap[entry.Water.Ch], entry.Index)
		}
	}
}

func batchSync() {
	var entries []*SyncEntry
	var loop uint64

	b := pstore.NewWriteBatch()
	defer b.Destroy()

	for {
		select {
		case e := <-syncCh:
			entries = append(entries, e)

		default:
			// default is executed if no other case is ready.
			start := time.Now()
			if len(entries) > 0 {
				x.AssertTrue(b != nil)
				loop++
				State().elog.Printf("[%4d] Writing schema batch of size: %v\n", loop, len(entries))
				for _, e := range entries {
					val, err := e.Schema.Marshal()
					x.Checkf(err, "Error while marshalling schema description")
					b.Put(x.SchemaKey(e.Attr), val)
				}
				x.Checkf(pstore.WriteBatch(b), "Error while writing to RocksDB.")
				b.Clear()

				entriesMap := make(map[chan x.Mark][]uint64)
				addToEntriesMap(entriesMap, entries)
				for ch, indices := range entriesMap {
					ch <- x.Mark{Indices: indices, Done: true}
				}
				entries = entries[:0]
			}
			// Add a sleep clause to avoid a busy wait loop if there's no input to commitCh.
			sleepFor := 10*time.Millisecond - time.Since(start)
			time.Sleep(sleepFor)
		}
	}
}
