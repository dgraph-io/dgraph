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

package uid

import (
	"context"
	"encoding/binary"
	"sync"
	"time"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	"golang.org/x/net/trace"
)

const (
	LeasePredicate = "_lease_"
)

var (
	lmgr     *lockManager
	leasemgr *leaseManager
	gid      uint32
	pstore   *store.Store
	leaseKey []byte
)

// Other alternatives could be
// 1. depending on number of xids in request, we take a lease for that
//    many xids. But the lease would need to be serialized and would be
//    a bottleneck
// 2. we take a lease for large number of xids, then we assign uids to
//    xids until we have sufficient number of uids. But we won't persist the
//    the  nextId, so we would loose uid range on restart and we would also
//     need to reset on leader change
type leaseManager struct {
	x.SafeMutex
	leasedId uint64
	nextId   uint64
	elog     trace.EventLog
}

func (l *leaseManager) init() {
	leasemgr.elog = trace.NewEventLog("Lease Manager", "")
	// uid 0 is not allowed
	leasemgr.nextId = 1
	leasemgr.leasedId = 0
}

func LeaseManager() *leaseManager {
	return leasemgr
}

func (l *leaseManager) Reload(group uint32) error {
	if group != gid {
		return nil
	}
	leasedId := getLease()
	l.Lock()
	defer l.Unlock()
	l.setLeasedId(leasedId)
	l.useAllIds()
	return nil
}

func (l *leaseManager) Update(ctx context.Context, leasedId uint64) error {
	l.Lock()
	defer l.Unlock()
	l.setLeasedId(leasedId)
	err := setLease(ctx, leasedId)
	l.elog.Printf("Updating leasedId: %d", leasedId)
	return err
}

// returns nextId and leasedId
func (l *leaseManager) Get() (uint64, uint64) {
	l.RLock()
	defer l.RUnlock()
	return l.nextId, l.leasedId
}

func (l *leaseManager) setLeasedId(leasedId uint64) {
	l.AssertLock()
	l.leasedId = leasedId
}

func (l *leaseManager) UseAllIds() {
	l.Lock()
	defer l.Unlock()
	l.nextId = l.leasedId + 1
	l.elog.Printf("Updating nextId : %d", l.nextId)
}

func (l *leaseManager) useAllIds() {
	l.AssertLock()
	l.nextId = l.leasedId + 1
}

// AssignNew assigns N unique uids sequentially
// and returns the starting number of the sequence
func (l *leaseManager) AssignNew(N uint64) uint64 {
	l.Lock()
	defer l.Unlock()
	x.AssertTruef(l.leasedId-l.nextId+1 >= N, "required number of uids not available")
	id := l.nextId
	l.nextId += N
	return id
}

type lockManager struct {
	sync.RWMutex
	uids map[string]time.Time
	ch   map[string][]chan XidAndUid
}

type XidAndUid struct {
	Xid string
	Uid uint64
}

func (lm *lockManager) init() {
	lm.uids = make(map[string]time.Time)
	lm.ch = make(map[string][]chan XidAndUid)
}

// CanProposeUid is used to take a lock over xid for proposing uid
func (lm *lockManager) CanProposeUid(xid string, ch chan XidAndUid) bool {
	lm.Lock()
	defer lm.Unlock()
	if _, has := lm.uids[xid]; has {
		lm.ch[xid] = append(lm.ch[xid], ch)
		return false
	}
	lm.uids[xid] = time.Now()
	return true
}

// Done sends notification on all registered channels
func (lm *lockManager) Done(xid string, uid uint64) {
	lm.Lock()
	defer lm.Unlock()
	for _, ch := range lm.ch[xid] {
		ch <- XidAndUid{Xid: xid, Uid: uid}
	}
	delete(lm.ch, xid)
	delete(lm.uids, xid)
}

func LockManager() *lockManager {
	return lmgr
}

// lock over xid would be removed in case request crashes before calling
// done
func (lm *lockManager) clean() {
	ticker := time.NewTicker(time.Minute * 10)
	for range ticker.C {
		now := time.Now()
		lm.Lock()
		for xid, ts := range lm.uids {
			// A minute is enough to avoid the race condition issue for
			// proposing different uid for same xid
			if now.Sub(ts) > time.Minute*10 {
				delete(lm.uids, xid)
				delete(lm.ch, xid)
			}
		}
		lm.Unlock()
	}
}

// package level init
func Init(ps *store.Store) {
	pstore = ps
	gid = group.BelongsTo(LeasePredicate)
	leaseKey = x.DataKey(LeasePredicate, 1)

	lmgr = new(lockManager)
	lmgr.init()
	go lmgr.clean()

	leasemgr = new(leaseManager)
	leasemgr.init()
}

func getLease() uint64 {
	plist, decr := posting.GetOrCreate(leaseKey, gid)
	defer decr()
	val, err := plist.Value()
	if err == posting.ErrNoValue {
		return 0
	}
	return binary.LittleEndian.Uint64(val.Value.([]byte))
}

func setLease(ctx context.Context, value uint64) error {
	var bs [8]byte
	binary.LittleEndian.PutUint64(bs[:], value)
	edge := &taskp.DirectedEdge{
		Entity:    1,
		Attr:      LeasePredicate,
		Value:     bs[:],
		ValueType: uint32(types.IntID),
		Op:        taskp.DirectedEdge_SET,
	}
	plist, decr := posting.GetOrCreate(leaseKey, gid)
	defer decr()
	return plist.AddMutationWithIndex(ctx, edge)
}
