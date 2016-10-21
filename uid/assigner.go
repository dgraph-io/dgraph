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

// allocateUniqueUid returns an integer in range:
// [minIdx, maxIdx] derived based on numInstances and instanceIdx.
// which hasn't already been allocated to other xids. It does this by
// taking the fingerprint of the xid appended with zero or more spaces
// until the obtained integer is unique.
func allocateUniqueUid(instanceIdx uint64, numInstances uint64) uint64 {
	mod := math.MaxUint64 / numInstances
	minIdx := instanceIdx * mod

	buf := make([]byte, 128)
	for {
		_, err := rand.Read(buf)
		x.Checkf(err, "rand.Read shouldn't throw an error")

		uidb := farm.Fingerprint64(buf) // Generate from hash.
		uid := (uidb % mod) + minIdx
		if uid == math.MaxUint64 || !lmgr.isNew(uid) {
			continue
		}

		// Check if this uid has already been allocated.
		key := posting.Key(uid, "_uid_")
		pl, decr := posting.GetOrCreate(key)
		defer decr()

		if pl.Length() == 0 {
			return uid
		}
	}
	log.Fatalf("This shouldn't be reached.")
	return 0
}

// AssignNew assigns N unique uids.
func AssignNew(N int, instanceIdx uint64, numInstances uint64) x.Mutations {
	var m x.Mutations
	for i := 0; i < N; i++ {
		uid := allocateUniqueUid(instanceIdx, numInstances)
		t := x.DirectedEdge{
			Entity:    uid,
			Attribute: "_uid_",
			Value:     []byte("_taken_"), // not txid
			Source:    "_assigner_",
			Timestamp: time.Now(),
		}
		m.Set = append(m.Set, t)
	}
	return m
}
