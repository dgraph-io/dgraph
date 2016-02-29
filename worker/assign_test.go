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
