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
	minLeaseNum    = uint64(10000)
)

var (
	leasemgr *leaseManager
	leaseKey []byte
)

func init() {
	leaseKey = x.DataKey(LeasePredicate, 1)

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
	plist := posting.GetOrCreate(leaseKey, leaseGid)
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
	if l.maxLeaseId == 0 {
		return 0, x.Errorf("Lease manager not initialized!")
	}
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
	proposal := &protos.Proposal{Mutations: mutations}
	node := groups().Node(leaseGid)
	x.AssertTruef(node != nil, "Node doesn't serve group %d", leaseGid)
	return node.ProposeAndWait(ctx, proposal)
}
