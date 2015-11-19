package posting

import (
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
	"github.com/google/flatbuffers/go"
)

func ProcessTask(query []byte) (result []byte, rerr error) {
	uo := flatbuffers.GetUOffsetT(query)
	q := new(task.Query)
	q.Init(query, uo)

	b := flatbuffers.NewBuilder(0)
	voffsets := make([]flatbuffers.UOffsetT, q.UidsLength())
	uoffsets := make([]flatbuffers.UOffsetT, q.UidsLength())

	attr := string(q.Attr())
	for i := 0; i < q.UidsLength(); i++ {
		uid := q.Uids(i)
		key := Key(uid, attr)
		pl := GetOrCreate(key)

		task.ValueStart(b)
		var valoffset flatbuffers.UOffsetT
		if val, err := pl.Value(); err != nil {
			valoffset = b.CreateByteVector(x.Nilbyte)
		} else {
			valoffset = b.CreateByteVector(val)
		}
		task.ValueAddVal(b, valoffset)
		voffsets[i] = task.ValueEnd(b)

		ulist := pl.GetUids()
		uoffsets[i] = x.UidlistOffset(b, ulist)
	}
	task.ResultStartValuesVector(b, len(voffsets))
	for i := len(voffsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(voffsets[i])
	}
	valuesVent := b.EndVector(len(voffsets))

	task.ResultStartUidmatrixVector(b, len(uoffsets))
	for i := len(uoffsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(uoffsets[i])
	}
	matrixVent := b.EndVector(len(uoffsets))

	task.ResultStart(b)
	task.ResultAddValues(b, valuesVent)
	task.ResultAddUidmatrix(b, matrixVent)
	rend := task.ResultEnd(b)
	b.Finish(rend)
	return b.Bytes[b.Head():], nil
}

func NewQuery(attr string, uids []uint64) []byte {
	b := flatbuffers.NewBuilder(0)
	task.QueryStartUidsVector(b, len(uids))
	for i := len(uids) - 1; i >= 0; i-- {
		b.PrependUint64(uids[i])
	}
	vend := b.EndVector(len(uids))

	ao := b.CreateString(attr)
	task.QueryStart(b)
	task.QueryAddAttr(b, ao)
	task.QueryAddUids(b, vend)
	qend := task.QueryEnd(b)
	b.Finish(qend)
	return b.Bytes[b.Head():]
}
