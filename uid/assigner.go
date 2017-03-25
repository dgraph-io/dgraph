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
	sync.RWMutex
	uids map[uint64]time.Time
}

// isNew checks that the passed uid is the first time lockManager has seen it.
// This avoids collisions where a uid which is proposed but still not pushed to a posting list,
// gets assigned to another entity.
func (lm *lockManager) isNew(uid uint64) bool {
	lm.Lock()
	defer lm.Unlock()
	if _, has := lm.uids[uid]; has {
		return false
	}
	lm.uids[uid] = time.Now()
	return true
}

func (lm *lockManager) clean() {
	ticker := time.NewTicker(time.Minute)
	for range ticker.C {
		now := time.Now()
		lm.Lock()
		for uid, ts := range lm.uids {
			// A minute is enough to avoid the race condition issue for
			// proposing the same UID for a different entity.
			if now.Sub(ts) > time.Minute {
				delete(lm.uids, uid)
			}
		}
		lm.Unlock()
	}
}

// package level init
func init() {
	rand.Seed(time.Now().UnixNano())
	lmgr = new(lockManager)
	lmgr.uids = make(map[uint64]time.Time)
	go lmgr.clean()
}

func allocateUniqueUid(group uint32) uint64 {
	buf := make([]byte, 128)
	for {
		_, err := rand.Read(buf)
		x.Checkf(err, "rand.Read shouldn't throw an error")

		uid := farm.Fingerprint64(buf) // Generate from hash.
		if uid == math.MaxUint64 || !lmgr.isNew(uid) {
			continue
		}

		// Check if this uid has already been allocated.
		key := x.DataKey("_uid_", uid)
		pl, decr := posting.GetOrCreate(key, group)
		defer decr()

		if pl.Length(0) == 0 {
			return uid
		}
	}
}

// AssignNew assigns N unique uids.
func AssignNew(N int, group uint32) *taskp.Mutations {
	set := make([]*taskp.DirectedEdge, N)
	for i := 0; i < N; i++ {
		uid := allocateUniqueUid(group)
		set[i] = &taskp.DirectedEdge{
			Entity: uid,
			Attr:   "_uid_",
			Value:  []byte("_"), // not txid
			Label:  "_assigner_",
			Op:     taskp.DirectedEdge_SET,
		}
	}
	return &taskp.Mutations{Edges: set}
}
