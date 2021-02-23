/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package worker

import (
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/golang/glog"
	"go.etcd.io/etcd/raft/raftpb"
	"go.uber.org/zap"
	"sort"
)

// Uint64Slice implements sort interface
type Uint64Slice []uint64

func (p Uint64Slice) Len() int           { return len(p) }
func (p Uint64Slice) Less(i, j int) bool { return p[i] < p[j] }
func (p Uint64Slice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

// getIDs returns an ordered set of IDs included in the given snapshot and
// the entries. The given snapshot/entries can contain three kinds of
// ID-related entry:
// - ConfChangeAddNode, in which case the contained ID will be added into the set.
// - ConfChangeRemoveNode, in which case the contained ID will be removed from the set.
// - ConfChangeAddLearnerNode, in which the contained ID will be added into the set.
func GetIDs(snap *raftpb.Snapshot, ents []raftpb.Entry) []uint64 {
	ids := make(map[uint64]bool)
	if snap != nil {
		for _, id := range snap.Metadata.ConfState.Nodes {
			ids[id] = true
		}
	}
	for _, e := range ents {
		if e.Type != raftpb.EntryConfChange {
			continue
		}
		var cc raftpb.ConfChange
		err := cc.Unmarshal(e.Data)
		if err != nil {
			glog.Errorf("confChange Unmarshal error, err: [%v]", err)
			continue
		}
		switch cc.Type {
		case raftpb.ConfChangeAddLearnerNode:
			ids[cc.NodeID] = true
		case raftpb.ConfChangeAddNode:
			ids[cc.NodeID] = true
		case raftpb.ConfChangeRemoveNode:
			delete(ids, cc.NodeID)
		case raftpb.ConfChangeUpdateNode:
			// do nothing
		default:
			glog.Fatal("unknown ConfChange Type", zap.String("type", cc.Type.String()))
		}
	}
	sids := make(Uint64Slice, 0, len(ids))
	for id := range ids {
		sids = append(sids, id)
	}
	sort.Sort(sids)
	return []uint64(sids)
}

// createConfigChangeEnts creates a series of Raft entries (i.e.
// EntryConfChange) to remove the set of given IDs from the cluster. The ID
// `self` is _not_ removed, even if present in the set.
// If `self` is not inside the given ids, it creates a Raft entry to add a
// default member with the given `self`.
func CreateConfigChangeEnts(ids []uint64, self uint64, term, index uint64,
	group uint32, selfRaftAddr string, isLearner bool) ([]raftpb.Entry, []*pb.Member, error) {
	found := false
	for _, id := range ids {
		if id == self {
			found = true
		}
	}

	var ents []raftpb.Entry
	var removeMembers []*pb.Member
	next := index + 1

	// NB: always add self first, then remove other nodes. Raft will panic if the
	// set of voters ever becomes empty.
	if !found {
		rc := pb.RaftContext{
			Id:        self,
			Addr:      selfRaftAddr,
			Group:     group,
			IsLearner: isLearner,
		}
		rcData, err := rc.Marshal()
		if err != nil {
			glog.Fatal("failed to marshal member", zap.Error(err))
		}
		cc := &raftpb.ConfChange{
			Type:    raftpb.ConfChangeAddNode,
			NodeID:  self,
			Context: rcData,
		}
		confChangeData, err := cc.Marshal()
		if err != nil {
			return nil, nil, err
		}
		e := raftpb.Entry{
			Type:  raftpb.EntryConfChange,
			Data:  confChangeData,
			Term:  term,
			Index: next,
		}
		ents = append(ents, e)
		next++
	}

	for _, id := range ids {
		if id == self {
			continue
		}
		cc := &raftpb.ConfChange{
			Type:   raftpb.ConfChangeRemoveNode,
			NodeID: id,
		}
		confChangeData, err := cc.Marshal()
		if err != nil {
			return nil, nil, err
		}
		e := raftpb.Entry{
			Type:  raftpb.EntryConfChange,
			Data:  confChangeData,
			Term:  term,
			Index: next,
		}
		ents = append(ents, e)
		next++

		removeMembers = append(removeMembers, &pb.Member{Id: id, GroupId: group, AmDead: true})
	}

	return ents, removeMembers, nil
}
