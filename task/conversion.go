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

package task

import (
	"encoding/binary"

	"github.com/dgraph-io/dgraph/protos/intern"
)

var (
	TrueVal  = FromBool(true)
	FalseVal = FromBool(false)
)

func FromInt(val int) *intern.TaskValue {
	bs := make([]byte, 4)
	binary.LittleEndian.PutUint32(bs, uint32(val))
	return &intern.TaskValue{Val: []byte(bs), ValType: int32(2)}
}

func ToInt(val *intern.TaskValue) int32 {
	result := binary.LittleEndian.Uint32(val.Val)
	return int32(result)
}

func FromBool(val bool) *intern.TaskValue {
	if val == true {
		return FromInt(1)
	}
	return FromInt(0)
}

func ToBool(val *intern.TaskValue) bool {
	if len(val.Val) == 0 {
		return false
	}
	result := ToInt(val)
	if result != 0 {
		return true
	}
	return false
}
