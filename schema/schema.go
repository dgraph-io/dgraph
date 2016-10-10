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

import "github.com/dgraph-io/dgraph/types"

// Item contains the name of the field and its type
type Item struct {
	Field string
	Typ   types.Type
}

var (
	// Map containing predicate to type information.
	str map[string]types.Type
	// Map containing fields that are indexed.
	indexedFields map[string]bool
)

func init() {
	str = make(map[string]types.Type)
	indexedFields = make(map[string]bool)
}

// IsIndexed returns if a given predicated is indexed or not.
func IsIndexed(str string) bool {
	return indexedFields[str]
}

// ScalarList returns the list of scalars in the geiven object.
func ScalarList(obj string) []Item {
	var res []Item
	objstr, ok := str[obj].(types.Object)
	if !ok {
		return res
	}
	for k, v := range objstr.Fields {
		if t, ok := getScalar(v); ok {
			res = append(res, Item{Field: k, Typ: t})
		}
	}
	return res
}

// TypeOf returns the type of given field.
func TypeOf(pred string) types.Type {
	if obj, ok := str[pred]; ok {
		return obj
	}
	if typ, ok := getScalar(pred); ok {
		return typ
	}
	return nil
}

func getScalar(typ string) (types.Type, bool) {
	return types.TypeForName(typ)
}
