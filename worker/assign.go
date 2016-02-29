package worker

import (
	"sync"

	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/uid"
	"github.com/google/flatbuffers/go"
)

func createXidListBuffer(xids map[string]bool) []byte {
	b := flatbuffers.NewBuilder(0)
	var offsets []flatbuffers.UOffsetT
	for xid := range xids {
		uo := b.CreateString(xid)
		offsets = append(offsets, uo)
	}

	task.XidListStartXidsVector(b, len(offsets))
	for _, uo := range offsets {
		b.PrependUOffsetT(uo)
	}
	ve := b.EndVector(len(offsets))

	task.XidListStart(b)
	task.XidListAddXids(b, ve)
	lo := task.XidListEnd(b)
	b.Finish(lo)
	return b.Bytes[b.Head():]
}

func getOrAssignUids(
	xidList *task.XidList) (uidList []byte, rerr error) {

	wg := new(sync.WaitGroup)
	uids := make([]uint64, xidList.XidsLength())
	che := make(chan error, xidList.XidsLength())
	for i := 0; i < xidList.XidsLength(); i++ {
		wg.Add(1)
		xid := string(xidList.Xids(i))

		go func() {
			defer wg.Done()
			u, err := uid.GetOrAssign(xid, 0, 1)
			if err != nil {
				che <- err
				return
			}
			uids[i] = u
		}()
	}
	wg.Wait()
	close(che)
	for err := range che {
		glog.WithError(err).Error("Encountered errors while getOrAssignUids")
		return uidList, err
	}

	b := flatbuffers.NewBuilder(0)
	task.UidListStartUidsVector(b, xidList.XidsLength())
	for i := len(uids) - 1; i >= 0; i-- {
		b.PrependUint64(uids[i])
	}
	ve := b.EndVector(xidList.XidsLength())

	task.UidListStart(b)
	task.UidListAddUids(b, ve)
	uend := task.UidListEnd(b)
	b.Finish(uend)
	return b.Bytes[b.Head():], nil
}
