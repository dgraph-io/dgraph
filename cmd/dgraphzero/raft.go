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
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/x"
	"golang.org/x/net/context"
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
	server *Server
	ctx    context.Context
	props  proposals
	leader uint32
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
		id := rand.Uint32()
		if n.props.Store(id, pctx) {
			proposal.Id = id
			break
		}
	}
	fmt.Printf(" ===> Proposal: %+v\n", proposal)
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
	errInvalidProposal = errors.New("Invalid group proposal")
)

func newGroup() *protos.Group {
	return &protos.Group{
		Members: make(map[uint64]*protos.Member),
		Tablets: make(map[string]*protos.Tablet),
	}
}

func (n *node) applyProposal(e raftpb.Entry) (uint32, error) {
	var p protos.ZeroProposal
	if err := p.Unmarshal(e.Data); err != nil {
		return 0, err
	}
	if p.Id == 0 {
		return 0, errInvalidProposal
	}
	x.Printf("Applying proposal: %+v\n", p)

	n.server.Lock()
	defer n.server.Unlock()

	state := n.server.state
	if p.Member != nil {
		if p.Member.GroupId == 0 {
			return 0, errInvalidProposal
		}
		group := state.Groups[p.Member.GroupId]
		if group == nil {
			group = newGroup()
			state.Groups[p.Member.GroupId] = group
		}
		_, has := group.Members[p.Member.Id]
		if !has && len(group.Members) >= n.server.NumReplicas {
			// We shouldn't allow more members than the number of replicas.
			return 0, errInvalidProposal
		}
		group.Members[p.Member.Id] = p.Member
	}
	if p.Tablet != nil {
		if p.Tablet.GroupId == 0 {
			return 0, errInvalidProposal
		}
		group := state.Groups[p.Tablet.GroupId]
		if group == nil {
			group = newGroup()
			state.Groups[p.Tablet.GroupId] = group
		}
		group.Tablets[p.Tablet.Predicate] = p.Tablet
	}
	if p.MaxLeaseId > 0 {
		state.MaxLeaseId = p.MaxLeaseId
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
}

func (n *node) initAndStartNode(wal *raftwal.Wal) error {
	_, restart, err := n.InitFromWal(wal)
	x.Check(err)

	if restart {
		x.Println("Restarting node for dgraphzero")
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
	// go n.snapshotPeriodically()
	go n.BatchAndSendMessages()
	return err
}

func (n *node) Run() {
	var leader bool
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	rcBytes, err := n.RaftContext.Marshal()
	x.Check(err)

	for {
		select {
		case <-ticker.C:
			n.Raft().Tick()

		case rd := <-n.Raft().Ready():
			// First store the entries, then the hardstate and snapshot.
			x.Check(n.Wal.Store(0, rd.HardState, rd.Entries))
			x.Check(n.Wal.StoreSnapshot(0, rd.Snapshot))

			// Now store them in the in-memory store.
			n.SaveToStorage(rd.Snapshot, rd.HardState, rd.Entries)

			if !raft.IsEmptySnap(rd.Snapshot) {
				var state protos.MembershipState
				x.Check(state.Unmarshal(rd.Snapshot.Data))
				n.server.Lock()
				n.server.state = &state
				n.server.Unlock()
			}

			for _, entry := range rd.CommittedEntries {
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
			n.Raft().Advance()
		}
	}
}
