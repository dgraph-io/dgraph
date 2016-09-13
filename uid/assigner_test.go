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
	"io/ioutil"
	"log"
	"math"
	"os"
	"testing"

	"github.com/Sirupsen/logrus"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/store"
)

// externalId returns the xid of a given uid by reading from the uidstore.
// It returns an error if there is no corresponding xid.
func externalId(uid uint64) (xid string, rerr error) {
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

func TestGetOrAssign(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)

	dir, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		t.Error(err)
		return
	}
	defer os.RemoveAll(dir)
	ps, err := store.NewStore(dir)
	if err != nil {
		t.Error(err)
		return
	}

	posting.Init()
	Init(ps)

	var u1, u2 uint64
	{
		uid, err := GetOrAssign("externalid0", 0, 1)
		if err != nil {
			t.Error(err)
		}
		t.Logf("Found uid: [%x]", uid)
		u1 = uid
	}

	{
		uid, err := GetOrAssign("externalid1", 0, 1)
		if err != nil {
			t.Error(err)
		}
		t.Logf("Found uid: [%x]", uid)
		u2 = uid
	}

	if u1 == u2 {
		t.Error("Uid1 and Uid2 shouldn't be the same")
	}
	// return

	{
		uid, err := GetOrAssign("externalid0", 0, 1)
		if err != nil {
			t.Error(err)
		}
		t.Logf("Found uid: [%x]", uid)
		if u1 != uid {
			t.Error("Uid should be the same.")
		}
	}
	// return

	{
		xid, err := externalId(u1)
		if err != nil {
			t.Error(err)
		}
		if xid != "externalid0" {
			t.Errorf("Expected externalid0. Found: [%q]", xid)
		}
	}
	return
}
