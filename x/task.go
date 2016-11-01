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

package x

import (
	"github.com/google/flatbuffers/go"

	"github.com/dgraph-io/dgraph/task"
)

type CountList struct {
	task.CountList
}

type ValueList struct {
	task.ValueList
}

func (l *CountList) MarshalBinary() ([]byte, error) {
	b := flatbuffers.NewBuilder(0)
	n := l.CountLength()
	task.CountListStartCountVector(b, n)
	for i := n - 1; i >= 0; i-- {
		b.PrependUint32(l.Count(i))
	}
	co := b.EndVector(n)
	task.CountListStart(b)
	task.CountListAddCount(b, co)
	b.Finish(task.CountListEnd(b))
	return b.FinishedBytes(), nil
}

func (l *CountList) UnmarshalBinary(data []byte) error {
	l.Init(data, 0)
	return nil
}

func (l *ValueList) MarshalBinary() ([]byte, error) {
	b := flatbuffers.NewBuilder(0)
	n := l.ValuesLength()

	voffsets := make([]flatbuffers.UOffsetT, n)
	var v task.Value
	for i := 0; i < n; i++ {
		Assert(l.Values(&v, i))
		valoffset := b.CreateByteVector(v.ValBytes())
		task.ValueStart(b)
		task.ValueAddVal(b, valoffset)
		task.ValueAddValType(b, v.ValType())
		voffsets[i] = task.ValueEnd(b)
	}

	task.ValueListStartValuesVector(b, n)
	for i := n - 1; i >= 0; i-- {
		b.PrependUOffsetT(voffsets[i])
	}
	valuesVecOffset := b.EndVector(n)

	task.ValueListStart(b)
	task.ValueListAddValues(b, valuesVecOffset)
	b.Finish(task.ValueListEnd(b))
	return b.FinishedBytes(), nil
}

func (l *ValueList) UnmarshalBinary(data []byte) error {
	l.Init(data, 0)
	return nil
}
