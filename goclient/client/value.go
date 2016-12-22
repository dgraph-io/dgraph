/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package client

import (
	"github.com/dgraph-io/dgraph/query/graph"
)

// Value represents a value sent in a mutation.
type Value *graph.Value

// Int returns an int graph.Value
func Int(val int32) Value {
	return &graph.Value{&graph.Value_IntVal{val}}
}

// Double returns an double graph.Value
func Double(val float64) Value {
	return &graph.Value{&graph.Value_DoubleVal{val}}
}

// Str returns an string graph.Value
func Str(val string) Value {
	return &graph.Value{&graph.Value_StrVal{val}}
}

// Bytes returns an byte array graph.Value
func Bytes(val []byte) Value {
	return &graph.Value{&graph.Value_BytesVal{val}}
}

// Bool returns an bool graph.Value
func Bool(val bool) Value {
	return &graph.Value{&graph.Value_BoolVal{val}}
}

// Geo returns a geo graph.Value
func Geo(val []byte) Value {
	return &graph.Value{&graph.Value_GeoVal{val}}
}

// ToValue converts val into the appropriate Value
func ToValue(val interface{}) Value {
	switch v := val.(type) {
	case int32:
		return Int(v)
	case string:
		return Str(v)
	case float64:
		return Double(v)
	case bool:
		return Bool(v)
	default:
		return nil
	}
}
