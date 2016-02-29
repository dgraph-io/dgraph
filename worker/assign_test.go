package worker

import (
	"testing"

	"github.com/dgraph-io/dgraph/task"
	"github.com/google/flatbuffers/go"
)

func TestXidListBuffer(t *testing.T) {
	xids := map[string]bool{
		"b.0453": true,
		"d.z1sz": true,
		"e.abcd": true,
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
		xids[xid] = false
	}
	for xid, untouched := range xids {
		if untouched {
			t.Errorf("Expected xid: %v to be part of the buffer.", xid)
		}
	}
}
