/*
 * Copyright 2015 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package uid

import (
	"log"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/dgryski/go-farm"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/task"
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
	for _ = range ticker.C {
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
		key := x.DataKey("_uid_", uid, nil) // TODO: Add pluginContexts.
		pl, decr := posting.GetOrCreate(key, group)
		defer decr()

		if pl.Length(0) == 0 {
			return uid
		}
	}
	log.Fatalf("This shouldn't be reached.")
	return 0
}

// AssignNew assigns N unique uids.
func AssignNew(N int, group uint32) *task.Mutations {
	set := make([]*task.DirectedEdge, N)
	for i := 0; i < N; i++ {
		uid := allocateUniqueUid(group)
		set[i] = &task.DirectedEdge{
			Entity: uid,
			Attr:   "_uid_",
			Value:  []byte("_"), // not txid
			Label:  "_assigner_",
			Op:     task.DirectedEdge_SET,
		}
	}
	return &task.Mutations{Edges: set}
}
