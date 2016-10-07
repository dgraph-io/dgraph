/*
 * Copyright 2016 Dgraph Labs, Inc.
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

package query

import (
	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/types"
)

// This file contains helper functions for converting scalar types to
// protobuf values.

func toProtoValue(v types.Value) *graph.Value {
	switch val := v.(type) {
	case types.Bytes:
		return &graph.Value{&graph.Value_BytesVal{val}}

	case types.String:
		return &graph.Value{&graph.Value_StrVal{string(val)}}

	case types.Int32:
		return &graph.Value{&graph.Value_IntVal{int32(val)}}

	case types.Float:
		return &graph.Value{&graph.Value_DoubleVal{float64(val)}}

	case types.Bool:
		return &graph.Value{&graph.Value_BoolVal{bool(val)}}

	case types.Geo:
		var b, _ = val.MarshalBinary()
		return &graph.Value{&graph.Value_GeoVal{b}}

	default:
		// A type that isn't supported in the proto
		return nil
	}
}
