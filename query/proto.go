/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package query

import (
	"time"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	geom "github.com/twpayne/go-geom"
)

// This file contains helper functions for converting scalar types to
// protobuf values.

func toProtoValue(v types.Val) *protos.Value {
	switch v.Tid {
	case types.StringID:
		return &protos.Value{&protos.Value_StrVal{v.Value.(string)}}

	case types.IntID:
		return &protos.Value{&protos.Value_IntVal{v.Value.(int64)}}

	case types.FloatID:
		return &protos.Value{&protos.Value_DoubleVal{v.Value.(float64)}}

	case types.BoolID:
		return &protos.Value{&protos.Value_BoolVal{v.Value.(bool)}}

	case types.DateID:
		val := v.Value.(time.Time)
		return &protos.Value{&protos.Value_StrVal{val.Format(time.RFC3339)}}

	case types.DateTimeID:
		val := v.Value.(time.Time)
		return &protos.Value{&protos.Value_StrVal{val.Format(time.RFC3339)}}

	case types.GeoID:
		b := types.ValueForType(types.BinaryID)
		src := types.ValueForType(types.GeoID)
		src.Value = v.Value.(geom.T)
		x.Check(types.Marshal(src, &b))
		return &protos.Value{&protos.Value_GeoVal{b.Value.([]byte)}}

	case types.PasswordID:
		return &protos.Value{&protos.Value_PasswordVal{v.Value.(string)}}

	case types.UidID:
		return &protos.Value{&protos.Value_UidVal{v.Value.(uint64)}}

	case types.DefaultID:
		return &protos.Value{&protos.Value_DefaultVal{v.Value.(string)}}

	default:
		// A type that isn't supported in the proto
		return nil
	}
}
