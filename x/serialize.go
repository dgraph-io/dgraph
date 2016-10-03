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

	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/task"
)

// ParseValueList parses bytes into task.ValueList.
func ParseValueList(q *task.ValueList, b []byte) {
	q.Init(b, flatbuffers.GetUOffsetT(b))
}

// ParseUidList parses bytes into task.UidList.
func ParseUidList(q *task.UidList, b []byte) {
	q.Init(b, flatbuffers.GetUOffsetT(b))
}

// ParseTaskQuery parses bytes into task.Query. This is very fast.
func ParseTaskQuery(q *task.Query, b []byte) {
	q.Init(b, flatbuffers.GetUOffsetT(b))
}

// ParseTaskResult parses bytes into task.Result. This is very fast.
func ParseTaskResult(r *task.Result, b []byte) {
	r.Init(b, flatbuffers.GetUOffsetT(b))
}

// ParsePosting parses bytes into types.Posting. This is very fast.
func ParsePosting(p *types.Posting, b []byte) {
	p.Init(b, flatbuffers.GetUOffsetT(b))
}

// UidlistOffset adds a UidList to buffer and returns the offset.
func UidlistOffset(b *flatbuffers.Builder, sorted []uint64) flatbuffers.UOffsetT {
	task.UidListStartUidsVector(b, len(sorted))
	for i := len(sorted) - 1; i >= 0; i-- {
		b.PrependUint64(sorted[i])
	}
	ulist := b.EndVector(len(sorted))
	task.UidListStart(b)
	task.UidListAddUids(b, ulist)
	return task.UidListEnd(b)
}
