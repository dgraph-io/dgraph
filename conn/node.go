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
	"sync"
	"time"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/x"
	"golang.org/x/net/context"
)

var (
	errNoPeerPoolEntry = fmt.Errorf("no peerPool entry")
	errNoPeerPool      = fmt.Errorf("no peerPool pool, could not connect")
)

type PeerPoolEntry struct {
	// Never the empty string.  Possibly a bogus address -- bad port number, the value
	// of *myAddr, or some screwed up Raft config.
	addr string
	// An owning reference to a pool for this peer (or nil if addr is sufficiently bogus).
	poolOrNil *Pool
}

// peerPool stores the peers' addresses and our connections to them.  It has exactly one
// entry for every peer other than ourselves.  Some of these peers might be unreachable or
// have bogus (but never empty) addresses.
type PeerPool struct {
	sync.RWMutex
	peers map[uint64]PeerPoolEntry
}

// getPool returns the non-nil pool for a peer.  This might error even if get(id)
// succeeds, if the pool is nil.  This happens if the peer was configured so badly (it had
// a totally bogus addr) we can't make a pool.  (A reasonable refactoring would have us
// make a pool, one that has a nil gRPC connection.)
//
// You must call pools().release on the pool.
func (p *PeerPool) getPool(id uint64) (*Pool, error) {
	p.RLock()
	defer p.RUnlock()
	ent, ok := p.peers[id]
	if !ok {
		return nil, errNoPeerPoolEntry
	}
	if ent.poolOrNil == nil {
		return nil, errNoPeerPool
	}
	ent.poolOrNil.AddOwner()
	return ent.poolOrNil, nil
}

func (p *PeerPool) get(id uint64) (string, bool) {
	p.RLock()
	defer p.RUnlock()
	ret, ok := p.peers[id]
	return ret.addr, ok
}

func (p *PeerPool) set(id uint64, addr string, pl *Pool) {
	p.Lock()
	defer p.Unlock()
	if old, ok := p.peers[id]; ok {
		if old.poolOrNil != nil {
			Get().Release(old.poolOrNil)
		}
	}
	p.peers[id] = PeerPoolEntry{addr, pl}
}

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
	peers       PeerPool
	messages    chan sendmsg
	RaftContext *protos.RaftContext
	Store       *raft.MemoryStorage
	Wal         *raftwal.Wal

	// applied is used to keep track of the applied RAFT proposals.
	// The stages are proposed -> committed (accepted by cluster) ->
	// applied (to PL) -> synced (to RocksDB).
	Applied x.WaterMark
}

func NewNode(rc *protos.RaftContext) *Node {
	peers := PeerPool{
		peers: make(map[uint64]PeerPoolEntry),
	}
	store := raft.NewMemoryStorage()
	n := &Node{
		Id:    rc.Id,
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
		peers:       peers,
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
	n._confState = cs
}

// ConfState would return the latest ConfState stored in node.
func (n *Node) ConfState() *raftpb.ConfState {
	n.RLock()
	defer n.RUnlock()
	return n._confState
}

// Never returns ("", true)
func (n *Node) GetPeer(pid uint64) (string, bool) {
	return n.peers.get(pid)
}

// You must call release on the pool.  Can error for some pid's for which GetPeer
// succeeds.
func (n *Node) GetPeerPool(pid uint64) (*Pool, error) {
	return n.peers.getPool(pid)
}

// addr must not be empty.
func (n *Node) SetPeer(pid uint64, addr string, poolOrNil *Pool) {
	x.AssertTruef(addr != "", "SetPeer for peer %d has empty addr.", pid)
	n.peers.set(pid, addr, poolOrNil)
}

func (n *Node) Send(m raftpb.Message) {
	x.AssertTruef(n.Id != m.To, "Seding message to itself")
	data, err := m.Marshal()
	x.Check(err)
	if m.Type != raftpb.MsgHeartbeat && m.Type != raftpb.MsgHeartbeatResp {
		x.Printf("\t\tSENDING: %v %v-->%v\n", m.Type, m.From, m.To)
	}
	select {
	case n.messages <- sendmsg{to: m.To, data: data}:
		// pass
	default:
		// TODO: It's bad to fail like this.
		x.Fatalf("Unable to push messages to channel in send")
	}
}

func (n *Node) SaveToStorage(s raftpb.Snapshot, h raftpb.HardState,
	es []raftpb.Entry) {
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

	if !raft.IsEmptyHardState(h) {
		n.Store.SetHardState(h)
	}
	n.Store.Append(es)
}

const (
	messageBatchSoftLimit = 10000000
)

func (n *Node) BatchAndSendMessages() {
	batches := make(map[uint64]*bytes.Buffer)
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
			data := make([]byte, buf.Len())
			copy(data, buf.Bytes())
			go n.doSendMessage(to, data)
			buf.Reset()
		}
	}
}

func (n *Node) doSendMessage(to uint64, data []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	pool, err := n.GetPeerPool(to)
	if err != nil {
		// No such peer exists or we got handed a bogus config (bad addr), so we
		// can't send messages to this peer.
		return
	}
	defer Get().Release(pool)
	client := pool.Get()

	c := protos.NewRaftClient(client)
	p := &protos.Payload{Data: data}

	ch := make(chan error, 1)
	go func() {
		_, err = c.RaftMessage(ctx, p)
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
	if paddr, ok := n.GetPeer(pid); ok && paddr == addr {
		// Already connected.
		return
	}
	// Here's what we do.  Right now peerPool maps peer node id's to addr values.  If
	// a *pool can be created, good, but if not, we still create a peerPoolEntry with
	// a nil *pool.
	if addr == n.MyAddr {
		// TODO: Note this fact in more general peer health info somehow.
		x.Printf("Peer %d claims same host as me\n", pid)
		n.SetPeer(pid, addr, nil)
		return
	}
	p := Get().Connect(addr)
	n.SetPeer(pid, addr, p)
}

func (n *Node) AddToCluster(ctx context.Context, pid uint64) error {
	addr, ok := n.GetPeer(pid)
	x.AssertTruef(ok, "Unable to find conn pool for peer: %d", pid)
	rc := &protos.RaftContext{
		Addr:  addr,
		Group: n.RaftContext.Group,
		Id:    pid,
	}
	rcBytes, err := rc.Marshal()
	x.Check(err)
	return n.Raft().ProposeConfChange(ctx, raftpb.ConfChange{
		ID:      pid,
		Type:    raftpb.ConfChangeAddNode,
		NodeID:  pid,
		Context: rcBytes,
	})
}

// TODO: Get rid of this in the upcoming changes.
var n_ *Node

func (w *RaftServer) GetNode() *Node {
	w.nodeLock.RLock()
	defer w.nodeLock.RUnlock()
	return n_
}

type RaftServer struct {
	nodeLock sync.RWMutex // protects Node.
	unused   *Node
}

func (w *RaftServer) JoinCluster(ctx context.Context,
	rc *protos.RaftContext) (*protos.Payload, error) {
	if ctx.Err() != nil {
		return &protos.Payload{}, ctx.Err()
	}
	// Commenting out the following checks for now, until we get rid of groups.
	// TODO: Uncomment this after groups is removed.
	// if rc.Group != w.GetNode().Group || rc.Id == w.GetNode().Id {
	// 	return &protos.Payload{}, x.Errorf(errorNodeIDExists)
	// }
	// TODO: Figure out what other conditions we need to check to reject a Join.

	// // Best effor reject
	// if _, found := groups().Server(rc.Id, rc.Group); found || rc.Id == Config.RaftId {
	// 	return &protos.Payload{}, x.Errorf(errorNodeIDExists)
	// }

	node := w.GetNode()
	if node == nil {
		return &protos.Payload{}, errNoNode
	}
	if _, ok := node.GetPeer(rc.Id); ok {
		return &protos.Payload{}, x.Errorf("Node id already part of group.")
	}
	node.Connect(rc.Id, rc.Addr)

	c := make(chan error, 1)
	go func() { c <- node.AddToCluster(ctx, rc.Id) }()

	select {
	case <-ctx.Done():
		return &protos.Payload{}, ctx.Err()
	case err := <-c:
		return &protos.Payload{}, err
	}
}

var (
	errNoNode = fmt.Errorf("No node has been set up yet")
)

func (w *RaftServer) applyMessage(ctx context.Context, msg raftpb.Message) error {
	var rc protos.RaftContext
	x.Check(rc.Unmarshal(msg.Context))
	// node := groups().Node(rc.Group)
	// if node == nil {
	// 	// Maybe we went down, went back up, reconnected, and got an RPC
	// 	// message before we set up Raft?
	// 	return errNoNode
	// }
	node := w.GetNode()
	if node == nil || node.Raft() == nil {
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
	query *protos.Payload) (*protos.Payload, error) {
	if ctx.Err() != nil {
		return &protos.Payload{}, ctx.Err()
	}

	for idx := 0; idx < len(query.Data); {
		x.AssertTruef(len(query.Data[idx:]) >= 4,
			"Slice left of size: %v. Expected at least 4.", len(query.Data[idx:]))

		sz := int(binary.LittleEndian.Uint32(query.Data[idx : idx+4]))
		idx += 4
		msg := raftpb.Message{}
		if idx+sz > len(query.Data) {
			return &protos.Payload{}, x.Errorf(
				"Invalid query. Specified size %v overflows slice [%v,%v)\n",
				sz, idx, len(query.Data))
		}
		if err := msg.Unmarshal(query.Data[idx : idx+sz]); err != nil {
			x.Check(err)
		}
		if msg.Type != raftpb.MsgHeartbeat && msg.Type != raftpb.MsgHeartbeatResp {
			x.Printf("RECEIVED: %v %v-->%v\n", msg.Type, msg.From, msg.To)
		}
		if err := w.applyMessage(ctx, msg); err != nil {
			return &protos.Payload{}, err
		}
		idx += sz
	}
	// fmt.Printf("Got %d messages\n", count)
	return &protos.Payload{}, nil
}
