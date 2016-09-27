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

type scalar struct {
	Field string
	Typ   Type
}

var store map[string]Type

func init() {
	store = make(map[string]Type)
}

// ScalarList returns the list of scalars in the geiven object.
func ScalarList(obj string) []scalar {
	var res []scalar
	objStore, ok := store[obj].(Object)
	if !ok {
		return res
	}
	for k, v := range objStore.Fields {
		if t, ok := getScalar(v); ok {
			res = append(res, scalar{Field: k, Typ: t})
		}
	}
	return res
}

// TypeOf returns the type of given field.
func TypeOf(pred string) Type {
	if obj, ok := store[pred]; ok {
		return obj
	}
	if typ, ok := getScalar(pred); ok {
		return typ
	}
	return nil
}

func getScalar(typ string) (Type, bool) {
	var res Type
	switch typ {
	case "int":
		res = intType
	case "float":
		res = floatType
	case "string":
		res = stringType
	case "bool":
		res = booleanType
	case "id":
		res = idType
	default:
		res = nil
	}

	if res != nil {
		return res, true
	}

	return res, false
}
