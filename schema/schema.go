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

type schemaInformation struct {
	x.SafeMutex
	// Map containing predicate to type information.
	sm map[string]*types.SchemaDescription
}

func (si *schemaInformation) updateIfMissing(se *SyncEntry) (types.TypeID, error) {
	si.Lock()
	defer si.Unlock()
	if si.sm == nil {
		si.sm = make(map[string]*types.SchemaDescription)
	}

	if oldVal, ok := si.sm[se.Attr]; ok && oldVal.ValueType != se.SchemaDescription.ValueType {
		return types.TypeID(oldVal.ValueType), x.Errorf("Schema for attr %s already set to %d", se.Attr, se.SchemaDescription.ValueType)
	}

	si.sm[se.Attr] = se.SchemaDescription
	se.Water.Ch <- x.Mark{Index: se.Index, Done: false}
	syncCh <- *se
	fmt.Printf("Setting schema for attr %s: %v\n", se.Attr, se.SchemaDescription.ValueType)
	return types.TypeID(se.SchemaDescription.ValueType), nil
}

func UpdateIfMissing(se *SyncEntry) (types.TypeID, error) {
	return str.updateIfMissing(se)
}

func (si *schemaInformation) setType(attr string, valueType types.TypeID) (types.TypeID, error) {
	if si.sm == nil {
		si.sm = make(map[string]*types.SchemaDescription)
		si.sm[attr] = &types.SchemaDescription{ValueType: uint32(valueType)}
	}

	if sd, ok := si.sm[attr]; ok {
		sd.ValueType = uint32(valueType)
	} else {
		si.sm[attr] = &types.SchemaDescription{ValueType: uint32(valueType)}
	}
	fmt.Printf("Setting schema for attr %s: %v\n", attr, valueType)
	return valueType, nil
}

func (si *schemaInformation) set(attr string, s *types.SchemaDescription) (types.TypeID, error) {
	if si.sm == nil {
		si.sm = make(map[string]*types.SchemaDescription)
	}

	si.sm[attr] = s
	fmt.Printf("Setting schema for attr %s: %v\n", attr, s.ValueType)
	return types.TypeID(s.ValueType), nil
}

func (si *schemaInformation) getTypeOf(pred string) (types.TypeID, error) {
	if typ, ok := si.sm[pred]; ok {
		return types.TypeID(typ.ValueType), nil
	}
	return types.TypeID(100), x.Errorf("Undefined predicate")
}

var (
	str *schemaInformation
	// Map predicate to tokenizer.
	indexedFields map[string]tok.Tokenizer
	// Map containing fields / predicates that are reversed.
	reversedFields map[string]bool
	pstore         *store.Store
	syncCh         chan SyncEntry
)

func Init(ps *store.Store, file string) error {
	pstore = ps
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

func init() {
	reset()
}

func getTypeValue(sd *types.SchemaDescription) []byte {
	buf, err := sd.Marshal()
	x.Checkf(err, "error while serilizing schema description")
	return buf
}

// not thread safe, call only during initilization at beginning
func LoadFromDb() error {
	prefix := x.SchemaPrefix()
	idxIt := pstore.NewIterator()
	defer idxIt.Close()

	for idxIt.Seek(prefix); idxIt.ValidForPrefix(prefix); idxIt.Next() {
		key := idxIt.Key().Data()
		attr := x.Parse(key).Attr
		slice := idxIt.Value()
		defer slice.Free() // there won't be too many slices
		var sd types.SchemaDescription
		x.Checkf(sd.Unmarshal(slice.Data()), "Error while loading schema from db")
		str.set(attr, &sd)
	}

	return nil
}

func reset() {
	str = new(schemaInformation)
	indexedFields = make(map[string]tok.Tokenizer)
	reversedFields = make(map[string]bool)
	syncCh = make(chan SyncEntry, 10000)
}

// IsIndexed returns if a given predicate is indexed or not.
func IsIndexed(attr string) bool {
	_, found := indexedFields[attr]
	return found
}

// Tokenizer returns tokenizer for given predicate.
func Tokenizer(attr string) tok.Tokenizer {
	return indexedFields[attr]
}

// IsReversed returns if a given predicate is reversed or not.
func IsReversed(str string) bool {
	return reversedFields[str]
}

// TypeOf returns the type of given field.
func TypeOf(pred string) (types.TypeID, error) {
	str.RLock()
	defer str.RUnlock()
	return str.getTypeOf(pred)
}

// IndexedFields returns a list of indexed fields.
func IndexedFields() []string {
	out := make([]string, 0, len(indexedFields))
	for k := range indexedFields {
		out = append(out, k)
	}
	return out
}

// The following logic is used to batch up all the writes to RocksDB.
type SyncEntry struct {
	Attr              string
	SchemaDescription *types.SchemaDescription
	Water             *x.WaterMark
	Index             uint64
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
					b.Put(x.SchemaKey(e.Attr), getTypeValue(e.SchemaDescription))
				}
				x.Checkf(pstore.WriteBatch(b), "Error while writing to RocksDB.")
				b.Clear()

				// can't batch this as entries might belong to different raft groups
				for _, e := range entries {
					if e.Water != nil {
						e.Water.Ch <- x.Mark{Index: e.Index, Done: true}
					}
				}
				entries = entries[:0]
			}
			// Add a sleep clause to avoid a busy wait loop if there's no input to commitCh.
			sleepFor := 10*time.Millisecond - time.Since(start)
			time.Sleep(sleepFor)
		}
	}
}
