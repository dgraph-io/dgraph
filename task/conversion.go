/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package task

import (
	"encoding/binary"

	"github.com/dgraph-io/dgraph/protos/pb"
)

var (
	// TrueVal is the pb.TaskValue value equivalent to the "true" boolean.
	TrueVal  = FromBool(true)
	// FalseVal is the pb.TaskValue value equivalent to the "false" boolean.
	FalseVal = FromBool(false)
)

// FromInt converts the given int value into a pb.TaskValue object.
func FromInt(val int) *pb.TaskValue {
	bs := make([]byte, 8)
	binary.LittleEndian.PutUint64(bs, uint64(val))
	return &pb.TaskValue{Val: []byte(bs), ValType: pb.Posting_INT}
}

// ToInt converts the given pb.TaskValue object into an integer.
func ToInt(val *pb.TaskValue) int64 {
	result := binary.LittleEndian.Uint64(val.Val)
	return int64(result)
}

// FromBool converts the given boolean in to a pb.TaskValue object.
func FromBool(val bool) *pb.TaskValue {
	if val {
		return FromInt(1)
	}
	return FromInt(0)
}

// ToBool converts the given pb.TaskValue object into a boolean.
func ToBool(val *pb.TaskValue) bool {
	if len(val.Val) == 0 {
		return false
	}
	result := ToInt(val)
	return result != 0
}
