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
	"errors"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgryski/go-farm"
)

var lmgr *lockManager
var uidStore *store.Store
var eidPool = sync.Pool{
	New: func() interface{} {
		return new(entry)
	},
}

type entry struct {
	sync.Mutex
	ts time.Time
}

func (e entry) isOld() bool {
	e.Lock()
	defer e.Unlock()
	return time.Now().Sub(e.ts) > time.Minute
}

type lockManager struct {
	sync.RWMutex
	locks map[string]*entry
}

func (lm *lockManager) newOrExisting(xid string) *entry {
	lm.RLock()
	if e, ok := lm.locks[xid]; ok {
		lm.RUnlock()
		return e
	}
	lm.RUnlock()

	lm.Lock()
	defer lm.Unlock()
	if e, ok := lm.locks[xid]; ok {
		return e
	}
	e := eidPool.Get().(*entry)
	e.ts = time.Now()
	lm.locks[xid] = e
	return e
}

func (lm *lockManager) clean() {
	ticker := time.NewTicker(time.Minute)
	for _ = range ticker.C {
		count := 0
		lm.Lock()
		for xid, e := range lm.locks {
			if e.isOld() {
				count += 1
				delete(lm.locks, xid)
				eidPool.Put(e)
			}
		}
		lm.Unlock()
		// A minute is enough to avoid the race condition issue for
		// uid allocation to an xid.
	}
}

// package level init
func init() {
	lmgr = new(lockManager)
	lmgr.locks = make(map[string]*entry)
	// TODO(manishrjain): This map should be cleaned up.
	// go lmgr.clean()
}

func Init(ps *store.Store) {
	uidStore = ps
}

func allocateUniqueUid(xid string, instanceIdx uint64,
	numInstances uint64) (uid uint64, rerr error) {

	mod := math.MaxUint64 / numInstances
	minIdx := instanceIdx * mod
	for sp := ""; ; sp += " " {
		txid := xid + sp

		uid1 := farm.Fingerprint64([]byte(txid)) // Generate from hash.
		uid = (uid1 % mod) + minIdx

		if uid == math.MaxUint64 {
			continue
		}

		// Check if this uid has already been allocated.
		key := posting.Key(uid, "_xid_") // uid -> "_xid_" -> xid
		pl := posting.GetOrCreate(key, uidStore)

		if pl.Length() > 0 {
			// Something already present here.
			var p types.Posting
			pl.Get(&p, 0)
			continue
		}

		// Uid hasn't been assigned yet.
		t := x.DirectedEdge{
			Value:     []byte(xid), // not txid
			Source:    "_assigner_",
			Timestamp: time.Now(),
		}
		rerr = pl.AddMutation(context.Background(), t, posting.Set)
		return uid, rerr
	}
	return 0, errors.New("Some unhandled route lead me here." +
		" Wake the stupid developer up.")
}

func assignNew(pl *posting.List, xid string, instanceIdx uint64,
	numInstances uint64) (uint64, error) {

	entry := lmgr.newOrExisting(xid)
	entry.Lock()
	entry.ts = time.Now()
	defer entry.Unlock()

	if pl.Length() > 1 {
		log.Fatalf("We shouldn't have more than 1 uid for xid: %v\n", xid)

	} else if pl.Length() > 0 {
		var p types.Posting
		if ok := pl.Get(&p, 0); !ok {
			return 0, errors.New("While retrieving entry from posting list.")
		}
		return p.Uid(), nil
	}

	// No current id exists. Create one.
	uid, err := allocateUniqueUid(xid, instanceIdx, numInstances)
	if err != nil {
		return 0, err
	}

	t := x.DirectedEdge{
		ValueId:   uid,
		Source:    "_assigner_",
		Timestamp: time.Now(),
	}
	rerr := pl.AddMutation(context.Background(), t, posting.Set)
	return uid, rerr
}

func StringKey(xid string) []byte {
	return []byte("_uid_|" + xid)
}

func Get(xid string) (uid uint64, rerr error) {
	key := StringKey(xid)
	pl := posting.GetOrCreate(key, uidStore)
	if pl.Length() == 0 {
		return 0, fmt.Errorf("xid: %v doesn't have any uid assigned.", xid)
	}
	if pl.Length() > 1 {
		log.Fatalf("We shouldn't have more than 1 uid for xid: %v\n", xid)
	}
	var p types.Posting
	if ok := pl.Get(&p, 0); !ok {
		return 0, fmt.Errorf("While retrieving entry from posting list")
	}
	return p.Uid(), nil
}

func GetOrAssign(xid string, instanceIdx uint64,
	numInstances uint64) (uid uint64, rerr error) {

	key := StringKey(xid)
	pl := posting.GetOrCreate(key, uidStore)
	if pl.Length() == 0 {
		return assignNew(pl, xid, instanceIdx, numInstances)

	} else if pl.Length() > 1 {
		log.Fatalf("We shouldn't have more than 1 uid for xid: %v\n", xid)

	} else {
		// We found one posting.
		var p types.Posting
		if ok := pl.Get(&p, 0); !ok {
			return 0, errors.New("While retrieving entry from posting list")
		}
		return p.Uid(), nil
	}
	return 0, errors.New("Some unhandled route lead me here." +
		" Wake the stupid developer up.")
}

func ExternalId(uid uint64) (xid string, rerr error) {
	key := posting.Key(uid, "_xid_") // uid -> "_xid_" -> xid
	pl := posting.GetOrCreate(key, uidStore)
	if pl.Length() == 0 {
		return "", errors.New("NO external id")
	}

	if pl.Length() > 1 {
		log.Fatalf("This shouldn't be happening. Uid: %v", uid)
		return "", errors.New("Multiple external ids for this uid.")
	}

	var p types.Posting
	if ok := pl.Get(&p, 0); !ok {
		return "", errors.New("While retrieving posting")
	}

	if p.Uid() != math.MaxUint64 {
		log.Fatalf("Value uid must be MaxUint64. Uid: %v", p.Uid())
	}
	return string(p.ValueBytes()), rerr
}
