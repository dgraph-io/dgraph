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
	"log"
	"time"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/x"
)

type node struct {
	raft      raft.Node
	confState *raftpb.ConfState
	server    *Server

	cfg   *raft.Config
	wal   *raftwal.Wal
	store *raft.MemoryStorage
}

// Raft would return back the raft.Node stored in the node.
func (n *node) Raft() raft.Node {
	return n.raft
}

func (n *node) saveToStorage(s raftpb.Snapshot, h raftpb.HardState,
	es []raftpb.Entry) {
	if !raft.IsEmptySnap(s) {
		le, err := n.store.LastIndex()
		if err != nil {
			log.Fatalf("While retrieving last index: %v\n", err)
		}
		if s.Metadata.Index <= le {
			return
		}

		if err := n.store.ApplySnapshot(s); err != nil {
			log.Fatalf("Applying snapshot: %v", err)
		}
	}

	if !raft.IsEmptyHardState(h) {
		n.store.SetHardState(h)
	}
	n.store.Append(es)
}

func (n *node) applyMembershipState(m protos.MembershipUpdate) error {
	srv := n.server
	srv.Lock()
	defer srv.Unlock()

	srv.groupMap = make(map[uint32]*Group)
	for _, member := range m.Members {
		if srv.hasMember(member) {
			// This seems like a duplicate.
			return errors.New("Duplicate member found")
		}
		group, has := srv.groupMap[member.GroupId]
		if !has {
			group = &Group{idMap: make(map[uint64]protos.Membership)}
			srv.groupMap[member.GroupId] = group
		}
		group.idMap[member.Id] = *member
	}
	for _, tablet := range m.Tablets {
		group, has := srv.groupMap[tablet.GroupId]
		if !has {
			return errors.New("Unassigned tablet found")
		}
		for _, t := range group.tablets {
			if t.Predicate == tablet.Predicate {
				return errors.New("Duplicate tablet found")
			}
		}
		group.tablets = append(group.tablets, *tablet)
		group.size += int64(tablet.Size())
	}
	return nil
}

func (n *node) applyConfChange(e raftpb.Entry) {
	var cc raftpb.ConfChange
	cc.Unmarshal(e.Data)

	if len(cc.Context) > 0 {
		var rc protos.RaftContext
		x.Check(rc.Unmarshal(cc.Context))
		conn.Get().Connect(rc.Addr)
	}

	cs := n.Raft().ApplyConfChange(cc)
	n.confState = cs
}

func (n *node) Run() {
	// var leader bool
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	rcBytes, err := n.raftContext.Marshal()
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
			x.Check(n.wal.Store(0, rd.HardState, rd.Entries))
			x.Check(n.wal.StoreSnapshot(0, rd.Snapshot))

			// Now store them in the in-memory store.
			n.saveToStorage(rd.Snapshot, rd.HardState, rd.Entries)

			if !raft.IsEmptySnap(rd.Snapshot) {
				var state protos.MembershipUpdate
				x.Check(state.Unmarshal(rd.Snapshot.Data))
				x.Check(n.applyMembershipState(state))
			}

			for _, entry := range rd.CommittedEntries {
				if entry.Type == raftpb.EntryConfChange {
					n.applyConfChange(entry)
				}
			}

			for _, msg := range rd.Messages {
				msg.Conext = rcBytes
				n.send(msg)
			}
		}
	}
}
