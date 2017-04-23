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
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/dgryski/go-farm"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/x"
)

var lmgr *lockManager

type lockManager struct {
	sync.Mutex
	//uids map[uint64]time.Time
	uids map[uint64]int64
}

// isNew checks that the passed uid is the first time lockManager has seen it.
// This avoids collisions where a uid which is proposed but still not pushed to a posting list,
// gets assigned to another entity.
func (lm *lockManager) isNew(uid uint64, now *int64) bool {
	lm.Lock()
	//defer lm.Unlock()
	if _, has := lm.uids[uid]; has {
		lm.Unlock()
		return false
	}
	//lm.uids[uid] = time.Now().Unix()
	lm.uids[uid] = *now
	lm.Unlock()
	return true
}

func (lm *lockManager) clean() {
	ticker := time.NewTicker(time.Minute)
	for range ticker.C {
		now := time.Now().Unix()
		max := int64(time.Minute)
		lm.Lock()
		fmt.Println("Map Size UIDs: ", len(lm.uids))
		for uid, ts := range lm.uids {
			// A minute is enough to avoid the race condition issue for
			// proposing the same UID for a different entity.
			//if now.Sub(ts) > time.Minute {
			if now-ts > max {
				delete(lm.uids, uid)
			}
		}
		fmt.Println("Map Size UIDs: ", len(lm.uids))
		lm.Unlock()
	}
}

// package level init
func init() {
	rand.Seed(time.Now().UnixNano())
	lmgr = new(lockManager)
	//lmgr.uids = make(map[uint64]time.Time)
	lmgr.uids = make(map[uint64]int64)
	go lmgr.clean()
}

func Init() {
	rand.Seed(10376732542)
	lmgr = new(lockManager)
	//lmgr.uids = make(map[uint64]time.Time)
	lmgr.uids = make(map[uint64]int64)
	go lmgr.clean()
}

func allocateUniqueUid(group uint32, now *int64, f *func()) uint64 {
	buf := make([]byte, 128)
	for {
		//_, err := rand.Read(buf)
		rand.Read(buf)
		//random.Read(buf)
		//x.Checkf(err, "rand.Read shouldn't throw an error")

		uid := farm.Fingerprint64(buf) // Generate from hash.
		if uid == math.MaxUint64 || !lmgr.isNew(uid, now) {
			continue
		}

		// Check if this uid has already been allocated.
		key := x.DataKey("_uid_", uid)
		pl, decr := posting.GetOrCreate(key, group)
		//*f = decr
		defer decr()

		if pl.Length(0) == 0 {
			return uid
		}

		// length := pl.Length(0)
		// decr()

		// if length == 0 {
		// 	return uid
		// }
		// return uid
	}
}

// AssignNew assigns N unique uids.
func AssignNew(N int, group uint32) *taskp.Mutations {
	//buf := make([]byte, 128)
	set := make([]*taskp.DirectedEdge, N)
	//random := rand.New(rand.NewSource(int64(rand.Uint64())))
	now := time.Now().Unix()
	var f func()
	//var now int64 = 0
	//random := rand.New(rand.NewSource(23423422))
	for i := 0; i < N; i++ {
		//uid := allocateUniqueUid(group, random, buf, &now)
		uid := allocateUniqueUid(group, &now, &f)
		//f()
		set[i] = &taskp.DirectedEdge{
			Entity: uid,
			Attr:   "_uid_",
			Value:  []byte{'_'}, //("_"), // not txid
			Label:  "_assigner_",
			Op:     taskp.DirectedEdge_SET,
		}
	}
	return &taskp.Mutations{Edges: set}
}
