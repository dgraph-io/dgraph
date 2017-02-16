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

	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

type state struct {
	x.SafeMutex
	// Map containing predicate to type information.
	schema map[string]*types.Schema
}

func (s *state) updateIfMissing(se *SyncEntry) (types.TypeID, error) {
	s.Lock()
	defer s.Unlock()

	if oldVal, ok := s.schema[se.Attr]; ok && oldVal.ValueType != se.SchemaDescription.ValueType {
		return types.TypeID(oldVal.ValueType), x.Errorf("Schema for attr %s already set to %d", se.Attr, se.SchemaDescription.ValueType)
	}

	s.schema[se.Attr] = se.SchemaDescription
	se.Water.Ch <- x.Mark{Index: se.Index, Done: false}
	syncCh <- *se

	fmt.Printf("Setting schema for attr %s: %v\n", se.Attr, se.SchemaDescription.ValueType)
	return types.TypeID(se.SchemaDescription.ValueType), nil
}

// UpdateIfMissing checks sets the schema if it doesn't exist
// already for given predicate
func UpdateIfMissing(se *SyncEntry) (types.TypeID, error) {
	return pstate().updateIfMissing(se)
}

func (s *state) setType(attr string, valueType types.TypeID) {
	s.Lock()
	defer s.Unlock()
	if schema, ok := s.schema[attr]; ok {
		schema.ValueType = uint32(valueType)
	} else {
		s.schema[attr] = &types.Schema{ValueType: uint32(valueType)}
	}
	fmt.Printf("Setting schema for attr %s: %v\n", attr, valueType)
}

func (s *state) setReverse(attr string, rev bool) {
	s.Lock()
	defer s.Unlock()
	if schema, ok := s.schema[attr]; ok {
		schema.Reverse = rev
	} else {
		s.schema[attr] = &types.Schema{Reverse: rev}
	}
}

func (s *state) setIndex(attr string, tokenizer string) {
	fmt.Println("Setting tokenizer", attr, tokenizer)
	s.Lock()
	defer s.Unlock()
	schema, ok := s.schema[attr]
	x.AssertTruef(ok, "schema state not found for %s", attr)
	schema.Tokenizer = tokenizer
}

func (s *state) set(attr string, schema *types.Schema) {
	s.Lock()
	defer s.Unlock()
	s.schema[attr] = schema
	fmt.Printf("Setting schema for attr %s: %v\n", attr, schema.ValueType)
}

func (s *state) typeOf(pred string) (types.TypeID, error) {
	s.RLock()
	defer s.RUnlock()
	if schema, ok := s.schema[pred]; ok {
		return types.TypeID(schema.ValueType), nil
	}
	return types.TypeID(100), x.Errorf("Undefined predicate")
}

func (s *state) isIndexed(pred string) bool {
	s.RLock()
	defer s.RUnlock()
	if schema, ok := s.schema[pred]; ok {
		return schema.Tokenizer != ""
	}
	return false
}

func (s *state) indexedFields() []string {
	s.RLock()
	defer s.RUnlock()
	var out []string
	for k, v := range s.schema {
		if v.Tokenizer != "" {
			out = append(out, k)
		}
	}
	return out
}

func (s *state) tokenizer(pred string) tok.Tokenizer {
	s.RLock()
	defer s.RUnlock()
	schema, ok := s.schema[pred]
	x.AssertTruef(ok, "schema state not found for %s", pred)
	return tok.GetTokenizer(schema.Tokenizer)
}

func (s *state) isReversed(pred string) bool {
	s.RLock()
	defer s.RUnlock()
	if schema, ok := s.schema[pred]; ok {
		return schema.Reverse
	}
	return false
}

var (
	predicatesState *state
	pstore          *store.Store
	syncCh          chan SyncEntry
)

func pstate() *state {
	return predicatesState
}

func Init(ps *store.Store, file string) error {
	pstore = ps
	reset()
	if len(file) > 0 {
		if err := parse(file); err != nil {
			return err
		}
	}
	if err := LoadFromDb(); err != nil {
		return err
	}
	go batchSync()
	return nil
}

// LoadFromDb reads schema information from db
// and stores it in memory
func LoadFromDb() error {
	prefix := x.SchemaPrefix()
	itr := pstore.NewIterator()
	defer itr.Close()

	for itr.Seek(prefix); itr.ValidForPrefix(prefix); itr.Next() {
		key := itr.Key().Data()
		attr := x.Parse(key).Attr
		data := itr.Value().Data()
		var s types.Schema
		x.Checkf(s.Unmarshal(data), "Error while loading schema from db")
		pstate().set(attr, &s)
	}

	return nil
}

func reset() {
	predicatesState = new(state)
	pstate().schema = make(map[string]*types.Schema)
	syncCh = make(chan SyncEntry, 10000)
}

// IsIndexed returns if a given predicate is indexed or not.
func IsIndexed(attr string) bool {
	return pstate().isIndexed(attr)
}

// Tokenizer returns tokenizer for given predicate.
func Tokenizer(attr string) tok.Tokenizer {
	return pstate().tokenizer(attr)
}

// IsReversed returns if a given predicate is reversed or not.
func IsReversed(str string) bool {
	return pstate().isReversed(str)
}

// TypeOf returns the type of given field.
func TypeOf(pred string) (types.TypeID, error) {
	return pstate().typeOf(pred)
}

// IndexedFields returns a list of indexed fields.
func IndexedFields() []string {
	return pstate().indexedFields()
}

// The following logic is used to batch up all the writes to RocksDB.
type SyncEntry struct {
	Attr              string
	SchemaDescription *types.Schema
	Water             *x.WaterMark
	Index             uint64
}

func addToEntriesMap(entriesMap map[chan x.Mark][]uint64, entries []SyncEntry) {
	for _, entry := range entries {
		if entry.Water != nil {
			entriesMap[entry.Water.Ch] = append(entriesMap[entry.Water.Ch], entry.Index)
		}
	}
}

func batchSync() {
	var entries []SyncEntry
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
				fmt.Printf("[%4d] Writing schema batch of size: %v\n", loop, len(entries))
				for _, e := range entries {
					val, err := e.SchemaDescription.Marshal()
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
