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
	"log"
	"math"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/store"
)

func getOrCreate(key []byte, ps *store.Store) *posting.List {
	l, _ := posting.GetOrCreate(key)
	return l
}

// externalId returns the xid of a given uid by reading from the uidstore.
// It returns an error if there is no corresponding xid.
func externalId(uid uint64) (xid string, rerr error) {
	key := posting.Key(uid, "_xid_") // uid -> "_xid_" -> xid
	pl := getOrCreate(key, uidStore)
	if pl.Length(0) == 0 {
		return "", errors.New("NO external id")
	}

	if pl.Length(0) > 1 {
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
