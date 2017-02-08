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
	"sync"

	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"

	"encoding/binary"
)

var (
	schemaLock sync.RWMutex
	// Map containing predicate to type information.
	str map[string]types.TypeID
	// Map predicate to tokenizer.
	indexedFields map[string]tok.Tokenizer
	// Map containing fields / predicates that are reversed.
	reversedFields map[string]bool
	pstore         *store.Store
)

func Init(ps *store.Store, file string) error {
	pstore = ps
	reset()
	if len(file) > 0 {
		if err := Parse(file); err != nil {
			return err
		}
	}
	if err := loadSchemaFromDb(); err != nil {
		return err
	}
	fmt.Printf("Loaded schema is %v\n", str)
	return nil
}

func MultiGet(attrs []string) map[string]string {
	schema := map[string]string{}
	if len(attrs) > 0 {
		for _, attr := range attrs {
			if schemaType, err := TypeOf(attr); err == nil {
				schema[attr] = schemaType.Name()
			}
		}
	} else {
		schemaLock.RLock()
		defer schemaLock.RUnlock()
		for k, v := range str {
			schema[k] = v.Name()
		}
	}
	return schema
}

func init() {
	reset()
}

// put in map and add to rocksdb only if it doesn't exist
func UpdateSchemaIfMissing(attr string, valueType types.TypeID) (types.TypeID, error) {
	schemaLock.Lock()
	defer schemaLock.Unlock()

	if oldVal, ok := str[attr]; ok && oldVal != valueType {
		return oldVal, x.Errorf("Schema for attr %s already set to %d", attr, valueType)
	}

	if err := setSchemaInDb(attr, valueType); err != nil {
		return types.TypeID(100), err
	}
	str[attr] = valueType
	fmt.Printf("Setting schema for attr %s: %v\n", attr, valueType)
	return valueType, nil
}

func setSchemaInDb(attr string, valueType types.TypeID) error {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(valueType))
	if err := pstore.SetOne(x.SchemaKey(attr), buf); err != nil {
		return err
	}
	return nil
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
		str[attr] = types.TypeID(binary.BigEndian.Uint32(slice.Data()))
	}

	return nil
}

func getSchemaFromDb(attr string) (types.TypeID, error) {
	if slice, err := pstore.Get(x.SchemaKey(attr)); err == nil && slice != nil {
		defer slice.Free()
		return types.TypeID(binary.BigEndian.Uint32(slice.Data()[1:])), nil
	} else {
		return types.TypeID(100), err
	}
}

func reset() {
	str = make(map[string]types.TypeID)
	indexedFields = make(map[string]tok.Tokenizer)
	reversedFields = make(map[string]bool)
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
	schemaLock.RLock()
	defer schemaLock.RUnlock()
	if typ, ok := str[pred]; ok {
		return typ, nil
	}
	return types.TypeID(100), x.Errorf("Undefined predicate")
}

// IndexedFields returns a list of indexed fields.
func IndexedFields() []string {
	out := make([]string, 0, len(indexedFields))
	for k := range indexedFields {
		out = append(out, k)
	}
	return out
}
