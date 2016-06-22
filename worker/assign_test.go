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

package worker

import (
	"testing"

	"github.com/dgraph-io/dgraph/task"
	"github.com/google/flatbuffers/go"
)

func TestXidListBuffer(t *testing.T) {
	xids := map[string]uint64{
		"b.0453": 0,
		"d.z1sz": 0,
		"e.abcd": 0,
	}

	buf := createXidListBuffer(xids)

	uo := flatbuffers.GetUOffsetT(buf)
	xl := new(task.XidList)
	xl.Init(buf, uo)

	if xl.XidsLength() != len(xids) {
		t.Errorf("Expected: %v. Got: %v", len(xids), xl.XidsLength())
	}
	for i := 0; i < xl.XidsLength(); i++ {
		xid := string(xl.Xids(i))
		t.Logf("Found: %v", xid)
		xids[xid] = 7
	}
	for xid, val := range xids {
		if val != 7 {
			t.Errorf("Expected xid: %v to be part of the buffer.", xid)
		}
	}
}
