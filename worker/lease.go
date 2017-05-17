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

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

const (
	LeasePredicate = "_lease_"
	minLeaseNum    = uint64(10000)
)

var (
	xidCache *xidToUids
	leasemgr *leaseManager
	leaseKey []byte
)

func init() {
	leaseKey = x.DataKey(LeasePredicate, 1)
	xidCache = new(xidToUids)
	xidCache.uid = make(map[string]uint64)

	leasemgr = new(leaseManager)
	leasemgr.elog = trace.NewEventLog("Lease Manager", "")
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
		l.minLeaseId = l.maxLeaseId + 1
		return
	}
	l.maxLeaseId = binary.LittleEndian.Uint64(val.Value.([]byte))
	l.minLeaseId = l.maxLeaseId + 1
	l.elog.Printf("reset lease, minLeasedId: %d maxLeasedId: %d\n", l.minLeaseId, l.maxLeaseId)
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
		if err := proposeAndWaitForLease(ctx, l.maxLeaseId+numLease); err != nil {
			return 0, err
		}
		l.maxLeaseId = l.maxLeaseId + numLease
	}

	id := l.minLeaseId // rand.Uint64()
	l.minLeaseId += N
	l.elog.Printf("assigned %d ids starting from %d\n", N, id)
	return id, nil
}

func proposeAndWaitForLease(ctx context.Context, maxLeaseId uint64) error {
	var bs [8]byte
	binary.LittleEndian.PutUint64(bs[:], maxLeaseId)
	edge := &protos.DirectedEdge{
		Entity:    1,
		Attr:      LeasePredicate,
		Value:     bs[:],
		ValueType: uint32(types.IntID),
		Op:        protos.DirectedEdge_SET,
	}
	mutations := &protos.Mutations{GroupId: leaseGid}
	mutations.Edges = append(mutations.Edges, edge)
	return proposeAndWait(ctx, mutations)
}

func proposeAndWait(ctx context.Context, mutations *protos.Mutations) error {
	proposal := &protos.Proposal{Mutations: mutations}
	node := groups().Node(leaseGid)
	x.AssertTruef(node != nil, "Node doesn't serve group %d", leaseGid)
	return node.ProposeAndWait(ctx, proposal)
}

/*
func proposeUid(ctx context.Context, xid string, uid uint64) error {
	edge := &protos.DirectedEdge{
		Entity:    uid,
		Attr:      "_xid_",
		Value:     []byte(xid),
		ValueType: uint32(types.StringID),
		Op:        protos.DirectedEdge_SET,
	}
	return proposeAndWait(ctx, edge)
}*/

type xidToUids struct {
	x.SafeMutex
	// replace with lru cache later
	uid map[string]uint64
}

func (xu *xidToUids) getUid(xid string) (uint64, error) {
	xu.RLock()
	if u, has := xu.uid[xid]; has {
		xu.RUnlock()
		return u, nil
	}
	xu.RUnlock()

	xu.Lock()
	defer xu.Unlock()
	if u, has := xu.uid[xid]; has {
		return u, nil
	}
	tokens, err := posting.IndexTokens("_xid_", "", types.Val{Tid: types.StringID, Value: []byte(xid)})
	if err != nil {
		return 0, err
	}
	x.AssertTrue(len(tokens) == 1)
	key := x.IndexKey("_xid_", tokens[0])
	pl, decr := posting.GetOrCreate(key, leaseGid)
	defer decr()
	ul := pl.Uids(emptyListOption)
	algo.ApplyFilter(ul, func(uid uint64, i int) bool {
		sv, err := fetchValue(uid, "_xid_", nil, types.StringID)
		if sv.Value == nil || err != nil {
			return false
		}
		return sv.Value.(string) == xid
	})
	if len(ul.Uids) == 0 {
		return 0, UidNotFound
	}
	x.AssertTrue(len(ul.Uids) == 1)
	xu.uid[xid] = ul.Uids[0]
	return xu.uid[xid], nil
}

func (x *xidToUids) setUid(xid string, uid uint64) {
	x.Lock()
	defer x.Unlock()
	x.uid[xid] = uid
}
