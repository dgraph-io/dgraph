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

package main

import (
	"errors"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/dgraph-io/badger/y"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/x"
	"golang.org/x/net/context"
	"golang.org/x/net/trace"
)

type proposalCtx struct {
	ch  chan error
	ctx context.Context
}

type proposals struct {
	sync.RWMutex
	ids map[uint32]*proposalCtx
}

func (p *proposals) Store(pid uint32, pctx *proposalCtx) bool {
	if pid == 0 {
		return false
	}
	p.Lock()
	defer p.Unlock()
	if p.ids == nil {
		p.ids = make(map[uint32]*proposalCtx)
	}
	if _, has := p.ids[pid]; has {
		return false
	}
	p.ids[pid] = pctx
	return true
}

func (p *proposals) Done(pid uint32, err error) {
	p.Lock()
	defer p.Unlock()
	pd, has := p.ids[pid]
	if !has {
		return
	}
	delete(p.ids, pid)
	pd.ch <- err
}

type node struct {
	*conn.Node
	server      *Server
	ctx         context.Context
	props       proposals
	leader      uint32
	subscribers map[uint32]chan struct{}
}

func (n *node) RegisterForUpdates(ch chan struct{}) uint32 {
	n.Lock()
	defer n.Unlock()
	if n.subscribers == nil {
		n.subscribers = make(map[uint32]chan struct{})
	}
	for {
		id := rand.Uint32()
		if _, has := n.subscribers[id]; has {
			continue
		}
		n.subscribers[id] = ch
		return id
	}
}

func (n *node) Deregister(id uint32) {
	n.Lock()
	defer n.Unlock()
	delete(n.subscribers, id)
}

func (n *node) triggerUpdates() {
	n.Lock()
	defer n.Unlock()
	for _, ch := range n.subscribers {
		select {
		case ch <- struct{}{}:
		// We can ignore it and don't send a notification, because they are going to
		// read a state version after now since ch is already full.
		default:
		}
	}
}

func (n *node) AmLeader() bool {
	return atomic.LoadUint32(&n.leader) == 1
}

func (n *node) proposeAndWait(ctx context.Context, proposal *protos.ZeroProposal) error {
	if n.Raft() == nil {
		return x.Errorf("Raft isn't initialized yet.")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	che := make(chan error, 1)
	pctx := &proposalCtx{
		ch:  che,
		ctx: ctx,
	}
	for {
		id := rand.Uint32() + 1
		if n.props.Store(id, pctx) {
			proposal.Id = id
			break
		}
	}
	data, err := proposal.Marshal()
	if err != nil {
		return err
	}

	// Propose the change.
	if err := n.Raft().Propose(ctx, data); err != nil {
		return x.Wrapf(err, "While proposing")
	}

	// Wait for proposal to be applied or timeout.
	select {
	case err := <-che:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

var (
	errInvalidProposal     = errors.New("Invalid group proposal")
	errTabletAlreadyServed = errors.New("Tablet is already being served")
)

func newGroup() *protos.Group {
	return &protos.Group{
		Members: make(map[uint64]*protos.Member),
		Tablets: make(map[string]*protos.Tablet),
	}
}

func (n *node) applyProposal(e raftpb.Entry) (uint32, error) {
	var p protos.ZeroProposal
	// Raft commits empty entry on becoming a leader.
	if len(e.Data) == 0 {
		return p.Id, nil
	}
	if err := p.Unmarshal(e.Data); err != nil {
		return p.Id, err
	}
	if p.Id == 0 {
		return 0, errInvalidProposal
	}

	n.server.Lock()
	defer n.server.Unlock()

	state := n.server.state
	state.Counter = e.Index
	if p.Member != nil {
		if p.Member.GroupId == 0 {
			return p.Id, errInvalidProposal
		}
		group := state.Groups[p.Member.GroupId]
		if group == nil {
			group = newGroup()
			state.Groups[p.Member.GroupId] = group
		}
		_, has := group.Members[p.Member.Id]
		if !has && len(group.Members) >= n.server.NumReplicas {
			// We shouldn't allow more members than the number of replicas.
			return p.Id, errInvalidProposal
		}
		group.Members[p.Member.Id] = p.Member
		// On replay of logs on restart we need to set nextGroup.
		if n.server.nextGroup <= p.Member.GroupId {
			n.server.nextGroup = p.Member.GroupId + 1
		}
	}
	if p.Tablet != nil {
		if p.Tablet.GroupId == 0 {
			return p.Id, errInvalidProposal
		}
		group := state.Groups[p.Tablet.GroupId]
		if group == nil {
			group = newGroup()
			state.Groups[p.Tablet.GroupId] = group
		}

		// There's a edge case that we're handling.
		// Two servers ask to serve the same tablet, then we need to ensure that
		// only the first one succeeds.
		if tablet := n.server.servingTablet(p.Tablet.Predicate); tablet != nil {
			if p.Tablet.Force {
				group := state.Groups[tablet.GroupId]
				delete(group.Tablets, p.Tablet.Predicate)
			} else {
				if tablet.GroupId != p.Tablet.GroupId {
					return p.Id, errTabletAlreadyServed
				}
				// This update can come from tablet size.
				p.Tablet.ReadOnly = tablet.ReadOnly
			}
		}
		group.Tablets[p.Tablet.Predicate] = p.Tablet
	}
	if p.MaxLeaseId > 0 {
		state.MaxLeaseId = p.MaxLeaseId
	} else {
		x.Printf("Applied proposal: %+v\n", p)
	}
	return p.Id, nil
}

func (n *node) applyConfChange(e raftpb.Entry) {
	var cc raftpb.ConfChange
	cc.Unmarshal(e.Data)

	if len(cc.Context) > 0 {
		var rc protos.RaftContext
		x.Check(rc.Unmarshal(cc.Context))
		n.Connect(rc.Id, rc.Addr)

		m := &protos.Member{Id: rc.Id, Addr: rc.Addr, GroupId: 0}
		n.server.storeZero(m)
	}

	cs := n.Raft().ApplyConfChange(cc)
	n.SetConfState(cs)
	select {
	case n.server.leaderChangeCh <- struct{}{}:
	default:
		// Ok to ingore
	}
}

func (n *node) initAndStartNode(wal *raftwal.Wal) error {
	idx, restart, err := n.InitFromWal(wal)
	n.Applied.SetDoneUntil(idx)
	x.Check(err)

	if restart {
		x.Println("Restarting node for dgraphzero")
		sp, err := n.Store.Snapshot()
		var state protos.MembershipState
		x.Check(state.Unmarshal(sp.Data))
		n.server.SetMembershipState(&state)

		x.Checkf(err, "Unable to get existing snapshot")
		n.SetRaft(raft.RestartNode(n.Cfg))

	} else if len(*peer) > 0 {
		p := conn.Get().Connect(*peer)
		if p == nil {
			return errInvalidAddress
		}

		gconn := p.Get()
		c := protos.NewRaftClient(gconn)
		err = errJoinCluster
		for err != nil {
			time.Sleep(time.Millisecond)
			_, err = c.JoinCluster(n.ctx, n.RaftContext)
		}
		n.SetRaft(raft.StartNode(n.Cfg, nil))

	} else {
		data, err := n.RaftContext.Marshal()
		x.Check(err)
		peers := []raft.Peer{{ID: n.Id, Context: data}}
		n.SetRaft(raft.StartNode(n.Cfg, peers))
	}

	go n.Run()
	go n.BatchAndSendMessages()
	return err
}

func (n *node) trySnapshot() {
	existing, err := n.Store.Snapshot()
	x.Checkf(err, "Unable to get existing snapshot")
	si := existing.Metadata.Index
	idx := n.Applied.DoneUntil()
	if idx <= si+1000 {
		return
	}

	data, err := n.server.MarshalMembershipState()
	x.Check(err)

	if tr, ok := trace.FromContext(n.ctx); ok {
		tr.LazyPrintf("Taking snapshot of state at watermark: %d\n", idx)
	}
	s, err := n.Store.CreateSnapshot(idx, n.ConfState(), data)
	x.Checkf(err, "While creating snapshot")
	x.Checkf(n.Store.Compact(idx), "While compacting snapshot")
	x.Check(n.Wal.StoreSnapshot(0, s))
}

func (n *node) Run() {
	var leader bool
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	rcBytes, err := n.RaftContext.Marshal()
	x.Check(err)

	// This chan could have capacity zero, because runReadIndexLoop never blocks without selecting
	// on readStateCh.  It's 2 so that sending rarely blocks (so the Go runtime doesn't have to
	// switch threads as much.)
	readStateCh := make(chan raft.ReadState, 2)
	closer := y.NewCloser(1)
	// We only stop runReadIndexLoop after the for loop below has finished interacting with it.
	// That way we know sending to readStateCh will not deadlock.
	defer closer.SignalAndWait()
	go n.RunReadIndexLoop(closer, readStateCh)

	for {
		select {
		case <-ticker.C:
			n.Raft().Tick()

		case rd := <-n.Raft().Ready():
			for _, rs := range rd.ReadStates {
				readStateCh <- rs
			}
			// First store the entries, then the hardstate and snapshot.
			x.Check(n.Wal.Store(0, rd.HardState, rd.Entries))
			x.Check(n.Wal.StoreSnapshot(0, rd.Snapshot))

			// Now store them in the in-memory store.
			n.SaveToStorage(rd.Snapshot, rd.HardState, rd.Entries)

			if !raft.IsEmptySnap(rd.Snapshot) {
				var state protos.MembershipState
				x.Check(state.Unmarshal(rd.Snapshot.Data))
				n.server.SetMembershipState(&state)
			}

			for _, entry := range rd.CommittedEntries {
				n.Applied.Begin(entry.Index)
				if entry.Type == raftpb.EntryConfChange {
					n.applyConfChange(entry)

				} else if entry.Type == raftpb.EntryNormal {
					pid, err := n.applyProposal(entry)
					if err != nil {
						x.Printf("While applying proposal: %v\n", err)
					}
					n.props.Done(pid, err)

				} else {
					x.Printf("Unhandled entry: %+v\n", entry)
				}
				n.Applied.Done(entry.Index)
			}

			// TODO: Should we move this to the top?
			if rd.SoftState != nil {
				if rd.RaftState == raft.StateLeader && !leader {
					n.server.updateNextLeaseId()
					leader = true
				}
				if leader {
					atomic.StoreUint32(&n.leader, 1)
				} else {
					atomic.StoreUint32(&n.leader, 0)
				}
			}

			for _, msg := range rd.Messages {
				msg.Context = rcBytes
				n.Send(msg)
			}
			if len(rd.CommittedEntries) > 0 {
				n.triggerUpdates()
			}
			n.trySnapshot()
			n.Raft().Advance()
		}
	}
}
