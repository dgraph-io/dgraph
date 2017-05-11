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
	"time"

	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

const (
	LeasePredicate = "_lease_"
	minLeaseNum    = uint64(500)
)

var (
	lmgr     *lockManager
	leasemgr *leaseManager
	leaseKey []byte
)

func init() {
	leaseKey = x.DataKey(LeasePredicate, 1)
	lmgr = new(lockManager)
	lmgr.uids = make(map[string]*uid)

	leasemgr = new(leaseManager)
	leasemgr.elog = trace.NewEventLog("Lease Manager", "")
	go lmgr.clean()
}

type leaseManager struct {
	x.SafeMutex
	minLeaseId uint64
	maxLeaseId uint64
	elog       trace.EventLog
}

// called on leader change
func (l *leaseManager) resetLease(group uint32) {
	if group != leaseGid {
		return
	}
	l.Lock()
	defer l.Unlock()
	plist, decr := posting.GetOrCreate(leaseKey, leaseGid)
	defer decr()
	val, err := plist.Value()
	if err == posting.ErrNoValue {
		// starts from two, 1 is used for storing lease data
		l.maxLeaseId = 1
	}
	l.maxLeaseId = binary.LittleEndian.Uint64(val.Value.([]byte))
	l.minLeaseId = l.maxLeaseId + 1
}

func leaseMgr() *leaseManager {
	return leasemgr
}

// AssignNew assigns N unique uids sequentially
// and returns the starting number of the sequence
func (l *leaseManager) assignNewUids(ctx context.Context, N uint64) (uint64, error) {
	x.AssertTrue(N > 0)
	l.Lock()
	defer l.Unlock()
	available := l.maxLeaseId - l.minLeaseId + 1
	numLease := minLeaseNum

	if N > numLease {
		numLease = N
	}
	if available < N {
		if err := proposeLease(ctx, l.maxLeaseId+numLease); err != nil {
			return 0, err
		}
		l.maxLeaseId = l.maxLeaseId + numLease
	}

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
	mutations := &protos.Mutations{GroupId: leaseGid}
	mutations.Edges = append(mutations.Edges, edge)
	proposal := &protos.Proposal{Mutations: mutations}
	node := groups().Node(leaseGid)
	x.AssertTruef(node != nil, "Node doesn't serve group %d", leaseGid)
	return node.ProposeAndWait(ctx, proposal)
}

type lockManager struct {
	x.SafeMutex
	uids map[string]*uid
}

type uid struct {
	x.SafeMutex
	uid uint64
	ts  time.Time
}

func (lm *lockManager) uid(xid string) *uid {
	lm.RLock()
	u, has := lm.uids[xid]
	if has {
		lm.RUnlock()
		return u
	}
	lm.RUnlock()

	lm.Lock()
	defer lm.Unlock()
	if u, has := lm.uids[xid]; has {
		return u
	}
	u = &uid{}
	u.ts = time.Now()
	lm.uids[xid] = u
	return u
}

func (lm *lockManager) assignUidForXid(ctx context.Context, xid string) (uint64, error) {
	s := lm.uid(xid)

	s.Lock()
	defer s.Unlock()
	if s.uid != 0 {
		return s.uid, nil
	}
	id, err := leaseMgr().assignNewUids(ctx, 1)
	if err != nil {
		return id, err
	}
	// To ensure that the key is not deleted immediately after persisting the id
	if err := proposeUid(ctx, xid, id); err != nil {
		return id, err
	}
	s.ts = time.Now()
	s.uid = id
	return s.uid, nil
}

func (lm *lockManager) clean() {
	ticker := time.NewTicker(time.Minute)
	for range ticker.C {
		now := time.Now()
		lm.Lock()
		for xid, u := range lm.uids {
			u.RLock()
			if now.Sub(u.ts) > 10*time.Minute {
				delete(lm.uids, xid)
			}
			u.RUnlock()
		}
		lm.Unlock()
	}
}

func lockMgr() *lockManager {
	return lmgr
}
