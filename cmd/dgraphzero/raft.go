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
	"time"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
)

type node struct {
	*conn.Node
	server *Server
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
	n.SetConfState(cs)
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
				msg.Context = rcBytes
				n.Send(msg)
			}
		}
	}
}
