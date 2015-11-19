/*
 * Copyright 2015 Manish R Jain <manishrjain@gmail.com>
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
	"bytes"
	"errors"
	"math"
	"sync"
	"time"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgryski/go-farm"
)

var glog = x.Log("uid")
var lmgr *lockManager

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
	e := new(entry)
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
			}
		}
		lm.Unlock()
		// A minute is enough to avoid the race condition issue for
		// uid allocation to an xid.
		glog.WithField("count", count).Info("Deleted old locks.")
	}
}

// package level init
func init() {
	lmgr = new(lockManager)
	lmgr.locks = make(map[string]*entry)
	// go lmgr.clean()
}

func allocateUniqueUid(xid string) (uid uint64, rerr error) {
	for sp := ""; ; sp += " " {
		txid := xid + sp
		uid = farm.Fingerprint64([]byte(txid)) // Generate from hash.
		glog.WithField("txid", txid).WithField("uid", uid).Debug("Generated")
		if uid == math.MaxUint64 {
			glog.Debug("Hit uint64max while generating fingerprint. Ignoring...")
			continue
		}

		// Check if this uid has already been allocated.
		key := posting.Key(uid, "_xid_") // uid -> "_xid_" -> xid
		pl := posting.GetOrCreate(key)

		if pl.Length() > 0 {
			// Something already present here.
			var p types.Posting
			pl.Get(&p, 0)

			var tmp interface{}
			posting.ParseValue(&tmp, p.ValueBytes())
			glog.Debug("Found existing xid: [%q]. Continuing...", tmp.(string))
			continue
		}

		// Uid hasn't been assigned yet.
		t := x.DirectedEdge{
			Value:     xid, // not txid
			Source:    "_assigner_",
			Timestamp: time.Now(),
		}
		rerr = pl.AddMutation(t, posting.Set)
		if rerr != nil {
			glog.WithError(rerr).Error("While adding mutation")
		}
		return uid, rerr
	}
	return 0, errors.New("Some unhandled route lead me here." +
		" Wake the stupid developer up.")
}

func assignNew(pl *posting.List, xid string) (uint64, error) {
	entry := lmgr.newOrExisting(xid)
	entry.Lock()
	entry.ts = time.Now()
	defer entry.Unlock()

	if pl.Length() > 1 {
		glog.Fatalf("We shouldn't have more than 1 uid for xid: %v\n", xid)

	} else if pl.Length() > 0 {
		var p types.Posting
		if ok := pl.Get(&p, 0); !ok {
			return 0, errors.New("While retrieving entry from posting list.")
		}
		return p.Uid(), nil
	}

	// No current id exists. Create one.
	uid, err := allocateUniqueUid(xid)
	if err != nil {
		return 0, err
	}

	t := x.DirectedEdge{
		ValueId:   uid,
		Source:    "_assigner_",
		Timestamp: time.Now(),
	}
	rerr := pl.AddMutation(t, posting.Set)
	return uid, rerr
}

func stringKey(xid string) []byte {
	buf := new(bytes.Buffer)
	buf.WriteString("_uid_")
	buf.WriteString("|")
	buf.WriteString(xid)
	return buf.Bytes()
}

func GetOrAssign(xid string) (uid uint64, rerr error) {
	key := stringKey(xid)
	pl := posting.GetOrCreate(key)
	if pl.Length() == 0 {
		return assignNew(pl, xid)

	} else if pl.Length() > 1 {
		glog.Fatalf("We shouldn't have more than 1 uid for xid: %v\n", xid)

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
	pl := posting.GetOrCreate(key)
	if pl.Length() == 0 {
		return "", errors.New("NO external id")
	}

	if pl.Length() > 1 {
		glog.WithField("uid", uid).Fatal("This shouldn't be happening.")
		return "", errors.New("Multiple external ids for this uid.")
	}

	var p types.Posting
	if ok := pl.Get(&p, 0); !ok {
		glog.WithField("uid", uid).Error("While retrieving posting")
		return "", errors.New("While retrieving posting")
	}

	if p.Uid() != math.MaxUint64 {
		glog.WithField("uid", uid).Fatal("Value uid must be MaxUint64.")
	}
	var t interface{}
	rerr = posting.ParseValue(&t, p.ValueBytes())
	xid = t.(string)
	return xid, rerr
}
