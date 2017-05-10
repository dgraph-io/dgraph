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

package worker

import (
	"context"
	"encoding/binary"

	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

const (
	LeasePredicate = "_lease_"
)

var (
	lmgr     *lockManager
	leasemgr *leaseManager
	gid      uint32
	leaseKey []byte
)

type leaseManager struct {
	x.SafeMutex
	minLeaseId uint64
	maxLeaseId uint64
	elog       trace.EventLog
}

// called on leader change
func (l *leaseManager) resetLease(group uint32) {
	if group != gid {
		return
	}
	l.Lock()
	defer l.Unlock()
	l.maxLeaseId = readMaxLease()
	l.minLeaseId = l.maxLeaseId + 1
}

func LeaseManager() *leaseManager {
	return leasemgr
}

func readMaxLease() uint64 {
	plist, decr := posting.GetOrCreate(leaseKey, gid)
	defer decr()
	val, err := plist.Value()
	if err == posting.ErrNoValue {
		return 0
	}
	return binary.LittleEndian.Uint64(val.Value.([]byte))
}

// AssignNew assigns N unique uids sequentially
// and returns the starting number of the sequence
func (l *leaseManager) assignNew(ctx context.Context, N uint64) (uint64, error) {
	x.AssertTrue(N > 0)
	l.Lock()
	defer l.Unlock()
	available := l.maxLeaseId - l.minLeaseId + 1
	numLease := uint64(500)

	if N > numLease {
		numLease = N
	}
	if available < N {
		if err := proposeLease(ctx, l.maxLeaseId+numLease); err != nil {
			return 0, err
		}
	}
	l.maxLeaseId = l.maxLeaseId + numLease

	id := l.minLeaseId
	l.minLeaseId += N
	return id, nil
}

func proposeLease(ctx context.Context, maxLeaseId uint64) error {
	var bs [8]byte
	binary.LittleEndian.PutUint64(bs[:], maxLeaseId)
	edge := &protos.DirectedEdge{
		Entity:    1,
		Attr:      LeasePredicate,
		Value:     bs[:],
		ValueType: uint32(types.IntID),
		Op:        protos.DirectedEdge_SET,
	}
	return propose(ctx, edge)
}

func proposeUid(ctx context.Context, xid string, uid uint64) error {
	edge := &protos.DirectedEdge{
		Entity:    uid,
		Attr:      "_xid_",
		Value:     []byte(xid),
		ValueType: uint32(types.StringID),
		Op:        protos.DirectedEdge_SET,
	}
	return propose(ctx, edge)
}

func propose(ctx context.Context, edge *protos.DirectedEdge) error {
	mutations := &protos.Mutations{GroupId: gid}
	mutations.Edges = append(mutations.Edges, edge)
	proposal := &protos.Proposal{Mutations: mutations}
	node := groups().Node(gid)
	x.AssertTruef(node != nil, "Node doesn't serve group %d", gid)
	return node.ProposeAndWait(ctx, proposal)
}

type lockManager struct {
	x.SafeMutex
	uids map[string]*Uid
}

type Uid struct {
	x.SafeMutex
	uid uint64
}

func (lm *lockManager) uid(xid string) *Uid {
	lm.RLock()
	u, has := lm.uids[xid]
	if has {
		lm.RUnlock()
		return u
	}
	lm.RUnlock()

	lm.Lock()
	defer lm.Unlock()
	u = &Uid{}
	lm.uids[xid] = u
	return u
}

func (lm *lockManager) getUid(ctx context.Context, xid string) (uint64, error) {
	s := lm.uid(xid)
	s.RLock()
	if s.uid != 0 {
		s.RUnlock()
		return s.uid, nil
	}
	s.RUnlock()

	s.Lock()
	defer s.Unlock()
	uid, err := GetUid(ctx, xid)
	if err == nil && uid != 0 {
		s.uid = uid
		return s.uid, nil
	}

	uid, err = LeaseManager().assignNew(ctx, 1)
	if err != nil {
		return uid, err
	}
	if err := proposeUid(ctx, xid, uid); err != nil {
		return uid, err
	}
	s.uid = uid
	return s.uid, nil
}

func (lm *lockManager) clean() {

}

func LockManager() *lockManager {
	return lmgr
}
