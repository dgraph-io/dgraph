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

	"encoding/binary"
)

type schemaInformation struct {
	x.SafeMutex
	// Map containing predicate to type information.
	sm map[string]types.TypeID
}

func (si *schemaInformation) updateSchemaIfMissing(se *schemaSyncEntry) (types.TypeID, error) {
	si.Lock()
	defer si.Unlock()
	if si.sm == nil {
		si.sm = make(map[string]types.TypeID)
	}

	if oldVal, ok := si.sm[se.Attr]; ok && oldVal != se.ValueType {
		return oldVal, x.Errorf("Schema for attr %s already set to %d", se.Attr, se.ValueType)
	}

	si.sm[se.Attr] = se.ValueType
	se.Water.Ch <- x.Mark{Index: se.Index, Done: false}
	syncCh <- *se
	fmt.Printf("Setting schema for attr %s: %v\n", se.Attr, se.ValueType)
	return se.ValueType, nil
}

func UpdateSchemaIfMissing(se *schemaSyncEntry) (types.TypeID, error) {
	return str.updateSchemaIfMissing(se)
}

// This function should be called only during init and is not thread safe
// This change won't be persisted to db, since this contains the schema read from file
func (si *schemaInformation) updateSchema(attr string, valueType types.TypeID) (types.TypeID, error) {
	if si.sm == nil {
		si.sm = make(map[string]types.TypeID)
	}

	si.sm[attr] = valueType
	fmt.Printf("Setting schema for attr %s: %v\n", attr, valueType)
	return valueType, nil
}

// This function should be called only during init and is not thread safe
func (si *schemaInformation) getSchema(pred string) (types.TypeID, bool) {
	typ, ok := si.sm[pred]
	return typ, ok
}

func (si *schemaInformation) getTypeOf(pred string) (types.TypeID, error) {
	si.AssertRLock()
	if typ, ok := si.sm[pred]; ok {
		return typ, nil
	}
	return types.TypeID(100), x.Errorf("Undefined predicate")
}

func (si *schemaInformation) getSchemaMap() map[string]string {
	si.RLock()
	defer si.RUnlock()
	if si.sm == nil {
		return nil
	}

	schema := map[string]string{}
	for k, v := range si.sm {
		schema[k] = v.Name()
	}
	return schema
}

var (
	str *schemaInformation
	// Map predicate to tokenizer.
	indexedFields map[string]tok.Tokenizer
	// Map containing fields / predicates that are reversed.
	reversedFields map[string]bool
	pstore         *store.Store
	syncCh         chan schemaSyncEntry
)

func Init(ps *store.Store, file string) error {
	pstore = ps
	if len(file) > 0 {
		if err := Parse(file); err != nil {
			return err
		}
	}
	if err := loadSchemaFromDb(); err != nil {
		return err
	}
	go batchSync()
	return nil
}

func MultiGet(attrs []string) map[string]string {
	str.RLock()
	defer str.RUnlock()
	var schema map[string]string
	if len(attrs) > 0 {
		schema = make(map[string]string)
		for _, attr := range attrs {
			if schemaType, err := str.getTypeOf(attr); err == nil {
				schema[attr] = schemaType.Name()
			}
		}
	} else {
		schema = str.getSchemaMap()
	}
	return schema
}

func init() {
	reset()
}

func getSchemaTypeValue(valueType types.TypeID) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(valueType))
	return buf
}

func loadSchemaFromDb() error {
	prefix := x.SchemaPrefix()
	idxIt := pstore.NewIterator()
	defer idxIt.Close()

	for idxIt.Seek(prefix); idxIt.ValidForPrefix(prefix); idxIt.Next() {
		key := idxIt.Key().Data()
		attr := x.ParseSchemaKey(key)
		slice := idxIt.Value()
		defer slice.Free() // there won't be too many slices
		str.updateSchema(attr, types.TypeID(binary.BigEndian.Uint32(slice.Data())))
	}

	return nil
}

func reset() {
	str = new(schemaInformation)
	indexedFields = make(map[string]tok.Tokenizer)
	reversedFields = make(map[string]bool)
	syncCh = make(chan schemaSyncEntry, 10000)
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
type schemaSyncEntry struct {
	Attr      string
	ValueType types.TypeID
	Water     *x.WaterMark
	Index     uint64
}

func batchSync() {
	var entries []schemaSyncEntry
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
				fmt.Printf("[%4d] Writing batch of size: %v\n", loop, len(entries))
				for _, e := range entries {
					b.Put(x.SchemaKey(e.Attr), getSchemaTypeValue(e.ValueType))
				}
				x.Checkf(pstore.WriteBatch(b), "Error while writing to RocksDB.")
				b.Clear()

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
