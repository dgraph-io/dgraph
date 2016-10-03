package worker

import (
	"context"
	"math"
	"sync"

	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
	flatbuffers "github.com/google/flatbuffers/go"
)

type groups struct {
	sync.RWMutex
	Map map[uint32]*node
}

var gr groups

func Node(groupId uint32) *node {
	gr.RLock()
	defer gr.RUnlock()
	n, has := gr.Map[groupId]
	x.Assertf(has, "Node should be present for group: %v", groupId)
	return n
}

func InitNode(groupId uint32, nodeId uint64, publicAddr string) *node {
	gr.Lock()
	defer gr.Unlock()
	if gr.Map == nil {
		gr.Map = make(map[uint32]*node)
	}

	node := newNode(groupId, nodeId, publicAddr)
	if _, has := gr.Map[groupId]; has {
		x.Assertf(false, "Didn't expect a node in RAFT group mapping: %v", groupId)
	}
	gr.Map[groupId] = node
	return node
}

func Inform(groupId uint32) {
	if groupId == math.MaxUint32 {
		return
	}
	node := Node(groupId)
	rc := task.GetRootAsRaftContext(node.raftContext, 0)

	var l byte
	if node.AmLeader() {
		l = byte(1)
	}

	b := flatbuffers.NewBuilder(0)
	so := b.CreateString(string(rc.Addr()))
	task.MembershipStart(b)
	task.MembershipAddGroup(b, groupId)
	task.MembershipAddAddr(b, so)
	task.MembershipAddLeader(b, l)
	uo := task.MembershipEnd(b)
	b.Finish(uo)
	data := b.Bytes[b.Head():]

	common := Node(math.MaxUint32)
	x.Checkf(common.ProposeAndWait(context.TODO(), membershipMsg, data),
		"Expected acceptance.")
}
