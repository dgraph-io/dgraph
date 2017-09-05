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
	"math/rand"
	"sync"
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
	p.Lock()
	defer p.Unlock()
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
}

func (n *node) proposeAndWait(ctx context.Context, proposal *protos.GroupProposal) error {
	if n.Raft() == nil {
		return x.Errorf("Raft isn't initialized yet.")
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	che := make(chan error, 1)
	pctx := proposalCtx{
		ch:  che,
		ctx: ctx,
	}
	for {
		id := rand.Uint32()
		if n.props.Store(id, pctx) {
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
	// Wait for proposal to be applied.
	return <-che
}

func (n *node) applyMembershipState(m protos.MembershipState) error {
	srv := n.server
	srv.Lock()
	defer srv.Unlock()
	srv.state = &m

	// srv.groupMap = make(map[uint32]*Group)
	// for _, member := range m.Members {
	// 	if srv.hasMember(member) {
	// 		// This seems like a duplicate.
	// 		return errors.New("Duplicate member found")
	// 	}
	// 	group, has := srv.groupMap[member.GroupId]
	// 	if !has {
	// 		group = &Group{idMap: make(map[uint64]protos.Member)}
	// 		srv.groupMap[member.GroupId] = group
	// 	}
	// 	group.idMap[member.Id] = *member
	// }
	// for _, tablet := range m.Tablets {
	// 	group, has := srv.groupMap[tablet.GroupId]
	// 	if !has {
	// 		return errors.New("Unassigned tablet found")
	// 	}
	// 	for _, t := range group.tablets {
	// 		if t.Predicate == tablet.Predicate {
	// 			return errors.New("Duplicate tablet found")
	// 		}
	// 	}
	// 	group.tablets = append(group.tablets, *tablet)
	// 	group.size += int64(tablet.Size())
	// }
	return nil
}

func (n *node) applyConfChange(e raftpb.Entry) {
	var cc raftpb.ConfChange
	cc.Unmarshal(e.Data)

	if len(cc.Context) > 0 {
		var rc protos.RaftContext
		x.Check(rc.Unmarshal(cc.Context))
		n.Connect(rc.Id, rc.Addr)
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
		defer conn.Get().Release(p)

		gconn := p.Get()
		c := protos.NewRaftClient(gconn)
		err = errJoinCluster
		for err != nil {
			time.Sleep(time.Millisecond)
			_, err = c.JoinCluster(n.ctx, n.RaftContext)
		}
		n.SetRaft(raft.StartNode(n.Cfg, nil))

	} else {
		peers := []raft.Peer{{ID: n.Id}}
		n.SetRaft(raft.StartNode(n.Cfg, peers))
	}

	go n.Run()
	// go n.snapshotPeriodically()
	go n.BatchAndSendMessages()
	return err
}

func (n *node) Run() {
	// var leader bool
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	rcBytes, err := n.RaftContext.Marshal()
	x.Check(err)

	for {
		select {
		case <-ticker.C:
			n.Raft().Tick()

		case rd := <-n.Raft().Ready():
			// if rd.SoftState != nil {
			// 	leader = rd.RaftState == raft.StateLeader
			// }
			// First store the entries, then the hardstate and snapshot.
			x.Check(n.Wal.Store(0, rd.HardState, rd.Entries))
			x.Check(n.Wal.StoreSnapshot(0, rd.Snapshot))

			// Now store them in the in-memory store.
			n.SaveToStorage(rd.Snapshot, rd.HardState, rd.Entries)

			if !raft.IsEmptySnap(rd.Snapshot) {
				var state protos.MembershipState
				x.Check(state.Unmarshal(rd.Snapshot.Data))
				x.Check(n.applyMembershipState(state))
			}

			for _, entry := range rd.CommittedEntries {
				if entry.Type == raftpb.EntryConfChange {
					n.applyConfChange(entry)
				} else {
					x.Printf("Unhandled entry: %+v\n", entry)
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
