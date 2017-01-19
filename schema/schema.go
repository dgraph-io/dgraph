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
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

var (
	// Map containing predicate to type information.
	str map[string]types.TypeID
	// Map predicate to tokenizer.
	indexedFields map[string]string
	// Map containing fields / predicates that are reversed.
	reversedFields map[string]bool
)

func init() {
	str = make(map[string]types.TypeID)
	indexedFields = make(map[string]string)
	reversedFields = make(map[string]bool)
}

// IsIndexed returns if a given predicate is indexed or not.
func IsIndexed(str string) bool {
	_, found := indexedFields[str]
	return found
}

// IsReversed returns if a given predicate is reversed or not.
func IsReversed(str string) bool {
	return reversedFields[str]
}

// TypeOf returns the type of given field.
func TypeOf(pred string) (types.TypeID, error) {
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
