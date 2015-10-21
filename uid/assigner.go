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
	"time"

	"github.com/dgryski/go-farm"
	"github.com/manishrjain/dgraph/posting"
	"github.com/manishrjain/dgraph/posting/types"
	"github.com/manishrjain/dgraph/store"
	"github.com/manishrjain/dgraph/x"
)

var log = x.Log("uid")

type Assigner struct {
	mstore *store.Store
	pstore *store.Store
}

func (a *Assigner) Init(pstore, mstore *store.Store) {
	a.mstore = mstore
	a.pstore = pstore
}

func (a *Assigner) allocateNew(xid string) (uid uint64, rerr error) {
	for sp := ""; ; sp += " " {
		txid := xid + sp
		uid = farm.Fingerprint64([]byte(txid)) // Generate from hash.
		log.Debugf("txid: [%q] uid: [%x]", txid, uid)

		// Check if this uid has already been allocated.
		// TODO: Posting List shouldn't be created here.
		// Possibly, use some singular class to serve all the posting lists.
		pl := new(posting.List)
		key := store.Key("_xid_", uid) // uid -> "_xid_" -> xid
		pl.Init(key, a.pstore, a.mstore)

		if pl.Length() > 0 {
			// Something already present here.
			var p types.Posting
			pl.Get(&p, 0)

			var tmp string
			posting.ParseValue(&tmp, p)
			log.Debug("Found existing xid: [%q]. Continuing...", tmp)
			continue
		}

		// Uid hasn't been assigned yet.
		t := x.Triple{
			Value:     xid, // not txid
			Source:    "_assigner_",
			Timestamp: time.Now(),
		}
		rerr = pl.AddMutation(t, posting.Set)
		if rerr != nil {
			x.Err(log, rerr).Error("While adding mutation")
		}
		if err := pl.CommitIfDirty(); err != nil {
			x.Err(log, err).Error("While commiting")
		}
		return uid, rerr
	}
	return 0, errors.New("Some unhandled route lead me here." +
		" Wake the stupid developer up.")
}

func Key(xid string) []byte {
	buf := new(bytes.Buffer)
	buf.WriteString("_uid_")
	buf.WriteString("|")
	buf.WriteString(xid)
	return buf.Bytes()
}

func (a *Assigner) GetOrAssign(xid string) (uid uint64, rerr error) {
	key := Key(xid)
	pl := new(posting.List)
	pl.Init(key, a.pstore, a.mstore)
	if pl.Length() == 0 {
		// No current id exists. Create one.
		uid, err := a.allocateNew(xid)
		if err != nil {
			return 0, err
		}
		t := x.Triple{
			ValueId:   uid,
			Source:    "_assigner_",
			Timestamp: time.Now(),
		}
		rerr = pl.AddMutation(t, posting.Set)
		return uid, rerr

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

func (a *Assigner) ExternalId(uid uint64) (xid string, rerr error) {
	pl := new(posting.List)
	key := store.Key("_xid_", uid) // uid -> "_xid_" -> xid
	pl.Init(key, a.pstore, a.mstore)
	if pl.Length() == 0 {
		return "", errors.New("NO external id")
	}

	if pl.Length() > 1 {
		log.WithField("uid", uid).Fatal("This shouldn't be happening.")
		return "", errors.New("Multiple external ids for this uid.")
	}

	var p types.Posting
	if ok := pl.Get(&p, 0); !ok {
		log.WithField("uid", uid).Error("While retrieving posting")
		return "", errors.New("While retrieving posting")
	}

	if p.Uid() != math.MaxUint64 {
		log.WithField("uid", uid).Fatal("Value uid must be MaxUint64.")
	}
	rerr = posting.ParseValue(&xid, p)
	return xid, rerr
}
