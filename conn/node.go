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

package conn

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/x"
	"golang.org/x/net/context"
)

var (
	ErrDuplicateRaftId = x.Errorf("Node is already part of group")
)

type sendmsg struct {
	to   uint64
	data []byte
}

type Node struct {
	x.SafeMutex

	// SafeMutex is for fields which can be changed after init.
	_confState *raftpb.ConfState
	_raft      raft.Node

	// Fields which are never changed after init.
	Cfg         *raft.Config
	MyAddr      string
	Id          uint64
	peers       map[uint64]string
	confChanges map[uint64]chan error
	messages    chan sendmsg
	RaftContext *intern.RaftContext
	Store       *raft.MemoryStorage
	Wal         *raftwal.Wal

	// applied is used to keep track of the applied RAFT proposals.
	// The stages are proposed -> committed (accepted by cluster) ->
	// applied (to PL) -> synced (to BadgerDB).
	Applied x.WaterMark
}

func NewNode(rc *intern.RaftContext) *Node {
	store := raft.NewMemoryStorage()
	n := &Node{
		Id:    rc.Id,
		MyAddr: rc.Addr,
		Store: store,
		Cfg: &raft.Config{
			ID:              rc.Id,
			ElectionTick:    100, // 200 ms if we call Tick() every 20 ms.
			HeartbeatTick:   1,   // 20 ms if we call Tick() every 20 ms.
			Storage:         store,
			MaxSizePerMsg:   256 << 10,
			MaxInflightMsgs: 256,
			Logger:          &raft.DefaultLogger{Logger: x.Logger},
			// We use lease-based linearizable ReadIndex for performance, at the cost of
			// correctness.  With it, communication goes follower->leader->follower, instead of
			// follower->leader->majority_of_followers->leader->follower.  We lose correctness
			// because the Raft ticker might not arrive promptly, in which case the leader would
			// falsely believe that its lease is still good.
			CheckQuorum:    true,
			ReadOnlyOption: raft.ReadOnlyLeaseBased,
		},
		// processConfChange etc are not throttled so some extra delta, so that we don't
		// block tick when applyCh is full
		peers:       make(map[uint64]string),
		confChanges: make(map[uint64]chan error),
		RaftContext: rc,
		messages:    make(chan sendmsg, 100),
		Applied:     x.WaterMark{Name: fmt.Sprintf("Applied watermark")},
	}
	n.Applied.Init()
	// TODO: n_ = n is a hack. We should properly init node, and make it part of the server struct.
	// This can happen once we get rid of groups.
	n_ = n
	return n
}

// SetRaft would set the provided raft.Node to this node.
// It would check fail if the node is already set.
func (n *Node) SetRaft(r raft.Node) {
	n.Lock()
	defer n.Unlock()
	x.AssertTrue(n._raft == nil)
	n._raft = r
}

// Raft would return back the raft.Node stored in the node.
func (n *Node) Raft() raft.Node {
	n.RLock()
	defer n.RUnlock()
	return n._raft
}

// SetConfState would store the latest ConfState generated by ApplyConfChange.
func (n *Node) SetConfState(cs *raftpb.ConfState) {
	n.Lock()
	defer n.Unlock()
	x.Printf("Setting conf state to %+v\n", cs)
	n._confState = cs
}

func (n *Node) DoneConfChange(id uint64, err error) {
	n.Lock()
	defer n.Unlock()
	ch, has := n.confChanges[id]
	if !has {
		return
	}
	delete(n.confChanges, id)
	ch <- err
}

func (n *Node) storeConfChange(che chan error) uint64 {
	n.Lock()
	defer n.Unlock()
	id := rand.Uint64()
	_, has := n.confChanges[id]
	for has {
		id = rand.Uint64()
		_, has = n.confChanges[id]
	}
	n.confChanges[id] = che
	return id
}

// ConfState would return the latest ConfState stored in node.
func (n *Node) ConfState() *raftpb.ConfState {
	n.RLock()
	defer n.RUnlock()
	return n._confState
}

func (n *Node) Peer(pid uint64) (string, bool) {
	n.RLock()
	defer n.RUnlock()
	addr, ok := n.peers[pid]
	return addr, ok
}

// addr must not be empty.
func (n *Node) SetPeer(pid uint64, addr string) {
	x.AssertTruef(addr != "", "SetPeer for peer %d has empty addr.", pid)
	n.Lock()
	defer n.Unlock()
	n.peers[pid] = addr
}

func (n *Node) WaitForMinProposal(ctx context.Context, read *api.LinRead) error {
	if read == nil || read.Ids == nil {
		return nil
	}
	gid := n.RaftContext.Group
	min := read.Ids[gid]
	return n.Applied.WaitForMark(ctx, min)
}

func (n *Node) Send(m raftpb.Message) {
	x.AssertTruef(n.Id != m.To, "Sending message to itself")
	data, err := m.Marshal()
	x.Check(err)
	select {
	case n.messages <- sendmsg{to: m.To, data: data}:
		// pass
	default:
		// ignore
	}
}

func (n *Node) SaveSnapshot(s raftpb.Snapshot) {
	if !raft.IsEmptySnap(s) {
		le, err := n.Store.LastIndex()
		if err != nil {
			log.Fatalf("While retrieving last index: %v\n", err)
		}
		if s.Metadata.Index <= le {
			return
		}

		if err := n.Store.ApplySnapshot(s); err != nil {
			log.Fatalf("Applying snapshot: %v", err)
		}
	}
}

func (n *Node) SaveToStorage(h raftpb.HardState, es []raftpb.Entry) {
	if !raft.IsEmptyHardState(h) {
		n.Store.SetHardState(h)
	}
	n.Store.Append(es)
}

func (n *Node) InitFromWal(wal *raftwal.Wal) (idx uint64, restart bool, rerr error) {
	n.Wal = wal

	var sp raftpb.Snapshot
	sp, rerr = wal.Snapshot(n.RaftContext.Group)
	if rerr != nil {
		return
	}
	var term uint64
	if !raft.IsEmptySnap(sp) {
		x.Printf("Found Snapshot, Metadata: %+v\n", sp.Metadata)
		restart = true
		if rerr = n.Store.ApplySnapshot(sp); rerr != nil {
			return
		}
		term = sp.Metadata.Term
		idx = sp.Metadata.Index
	}

	var hd raftpb.HardState
	hd, rerr = wal.HardState(n.RaftContext.Group)
	if rerr != nil {
		return
	}
	if !raft.IsEmptyHardState(hd) {
		x.Printf("Found hardstate: %+v\n", hd)
		restart = true
		if rerr = n.Store.SetHardState(hd); rerr != nil {
			return
		}
	}

	var es []raftpb.Entry
	es, rerr = wal.Entries(n.RaftContext.Group, term, idx)
	if rerr != nil {
		return
	}
	x.Printf("Group %d found %d entries\n", n.RaftContext.Group, len(es))
	if len(es) > 0 {
		restart = true
	}
	rerr = n.Store.Append(es)
	return
}

const (
	messageBatchSoftLimit = 10000000
)

func (n *Node) BatchAndSendMessages() {
	batches := make(map[uint64]*bytes.Buffer)
	failedConn := make(map[uint64]bool)
	for {
		totalSize := 0
		sm := <-n.messages
	slurp_loop:
		for {
			var buf *bytes.Buffer
			if b, ok := batches[sm.to]; !ok {
				buf = new(bytes.Buffer)
				batches[sm.to] = buf
			} else {
				buf = b
			}
			totalSize += 4 + len(sm.data)
			x.Check(binary.Write(buf, binary.LittleEndian, uint32(len(sm.data))))
			x.Check2(buf.Write(sm.data))

			if totalSize > messageBatchSoftLimit {
				// We limit the batch size, but we aren't pushing back on
				// n.messages, because the loop below spawns a goroutine
				// to do its dirty work.  This is good because right now
				// (*node).send fails(!) if the channel is full.
				break
			}

			select {
			case sm = <-n.messages:
			default:
				break slurp_loop
			}
		}

		for to, buf := range batches {
			if buf.Len() == 0 {
				continue
			}

			addr, has := n.Peer(to)
			pool, err := Get().Get(addr)
			if !has || err != nil {
				if exists := failedConn[to]; !exists {
					// So that we print error only the first time we are not able to connect.
					// Otherwise, the log is polluted with multiple errors.
					x.Printf("No healthy connection found to node Id: %d, err: %v\n", to, err)
					failedConn[to] = true
				}
				continue
			}

			failedConn[to] = false
			data := make([]byte, buf.Len())
			copy(data, buf.Bytes())
			go n.doSendMessage(pool, data)
			buf.Reset()
		}
	}
}

func (n *Node) doSendMessage(pool *Pool, data []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	client := pool.Get()

	c := intern.NewRaftClient(client)
	p := &api.Payload{Data: data}

	ch := make(chan error, 1)
	go func() {
		_, err := c.RaftMessage(ctx, p)
		if err != nil {
			x.Printf("Error while sending message to node with addr: %s, err: %v\n", pool.Addr, err)
		}
		ch <- err
	}()

	select {
	case <-ctx.Done():
		return
	case <-ch:
		// We don't need to do anything if we receive any error while sending message.
		// RAFT would automatically retry.
		return
	}
}

// Connects the node and makes its peerPool refer to the constructed pool and address
// (possibly updating ourselves from the old address.)  (Unless pid is ourselves, in which
// case this does nothing.)
func (n *Node) Connect(pid uint64, addr string) {
	if pid == n.Id {
		return
	}
	if paddr, ok := n.Peer(pid); ok && paddr == addr {
		// Already connected.
		return
	}
	// Here's what we do.  Right now peerPool maps peer node id's to addr values.  If
	// a *pool can be created, good, but if not, we still create a peerPoolEntry with
	// a nil *pool.
	if addr == n.MyAddr {
		// TODO: Note this fact in more general peer health info somehow.
		x.Printf("Peer %d claims same host as me\n", pid)
		n.SetPeer(pid, addr)
		return
	}
	Get().Connect(addr)
	n.SetPeer(pid, addr)
}

func (n *Node) DeletePeer(pid uint64) {
	if pid == n.Id {
		return
	}
	n.Lock()
	defer n.Unlock()
	delete(n.peers, pid)
}

func (n *Node) AddToCluster(ctx context.Context, pid uint64) error {
	addr, ok := n.Peer(pid)
	x.AssertTruef(ok, "Unable to find conn pool for peer: %d", pid)
	rc := &intern.RaftContext{
		Addr:  addr,
		Group: n.RaftContext.Group,
		Id:    pid,
	}
	rcBytes, err := rc.Marshal()
	x.Check(err)

	ch := make(chan error, 1)
	id := n.storeConfChange(ch)
	err = n.Raft().ProposeConfChange(ctx, raftpb.ConfChange{
		ID:      id,
		Type:    raftpb.ConfChangeAddNode,
		NodeID:  pid,
		Context: rcBytes,
	})
	if err != nil {
		return err
	}
	err = <-ch
	return err
}

func (n *Node) ProposePeerRemoval(ctx context.Context, id uint64) error {
	if n.Raft() == nil {
		return errNoNode
	}
	if _, ok := n.Peer(id); !ok && id != n.RaftContext.Id {
		return x.Errorf("Node %d not part of group", id)
	}
	ch := make(chan error, 1)
	pid := n.storeConfChange(ch)
	err := n.Raft().ProposeConfChange(ctx, raftpb.ConfChange{
		ID:     pid,
		Type:   raftpb.ConfChangeRemoveNode,
		NodeID: id,
	})
	if err != nil {
		return err
	}
	err = <-ch
	return err
}

// TODO: Get rid of this in the upcoming changes.
var n_ *Node

func (w *RaftServer) GetNode() *Node {
	w.nodeLock.RLock()
	defer w.nodeLock.RUnlock()
	return w.Node
}

type RaftServer struct {
	nodeLock sync.RWMutex // protects Node.
	Node     *Node
}

func (w *RaftServer) JoinCluster(ctx context.Context,
	rc *intern.RaftContext) (*api.Payload, error) {
	if ctx.Err() != nil {
		return &api.Payload{}, ctx.Err()
	}
	// Commenting out the following checks for now, until we get rid of groups.
	// TODO: Uncomment this after groups is removed.
	node := w.GetNode()
	if node == nil || node.Raft() == nil {
		return nil, errNoNode
	}
	// Check that the new node is from the same group as me.
	if rc.Group != node.RaftContext.Group {
		return nil, x.Errorf("Raft group mismatch")
	}
	// Also check that the new node is not me.
	if rc.Id == node.RaftContext.Id {
		return nil, ErrDuplicateRaftId
	}
	// Check that the new node is not already part of the group.
	if addr, ok := node.peers[rc.Id]; ok && rc.Addr != addr {
		Get().Connect(addr)
		// There exists a healthy connection to server with same id.
		if _, err := Get().Get(addr); err == nil {
			return &api.Payload{}, ErrDuplicateRaftId
		}
	}
	node.Connect(rc.Id, rc.Addr)

	c := make(chan error, 1)
	go func() { c <- node.AddToCluster(ctx, rc.Id) }()

	select {
	case <-ctx.Done():
		return &api.Payload{}, ctx.Err()
	case err := <-c:
		return &api.Payload{}, err
	}
}

var (
	errNoNode = fmt.Errorf("No node has been set up yet")
)

func (w *RaftServer) applyMessage(ctx context.Context, msg raftpb.Message) error {
	var rc intern.RaftContext
	x.Check(rc.Unmarshal(msg.Context))

	node := w.GetNode()
	if node == nil || node.Raft() == nil {
		return errNoNode
	}
	if rc.Group != node.RaftContext.Group {
		return errNoNode
	}
	node.Connect(msg.From, rc.Addr)

	c := make(chan error, 1)
	go func() { c <- node.Raft().Step(ctx, msg) }()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-c:
		return err
	}
}
func (w *RaftServer) RaftMessage(ctx context.Context,
	query *api.Payload) (*api.Payload, error) {
	if ctx.Err() != nil {
		return &api.Payload{}, ctx.Err()
	}

	for idx := 0; idx < len(query.Data); {
		x.AssertTruef(len(query.Data[idx:]) >= 4,
			"Slice left of size: %v. Expected at least 4.", len(query.Data[idx:]))

		sz := int(binary.LittleEndian.Uint32(query.Data[idx : idx+4]))
		idx += 4
		msg := raftpb.Message{}
		if idx+sz > len(query.Data) {
			return &api.Payload{}, x.Errorf(
				"Invalid query. Specified size %v overflows slice [%v,%v)\n",
				sz, idx, len(query.Data))
		}
		if err := msg.Unmarshal(query.Data[idx : idx+sz]); err != nil {
			x.Check(err)
		}
		if err := w.applyMessage(ctx, msg); err != nil {
			return &api.Payload{}, err
		}
		idx += sz
	}
	// fmt.Printf("Got %d messages\n", count)
	return &api.Payload{}, nil
}

// Hello rpc call is used to check connection with other workers after worker
// tcp server for this instance starts.
func (w *RaftServer) Echo(ctx context.Context, in *api.Payload) (*api.Payload, error) {
	return &api.Payload{Data: in.Data}, nil
}
