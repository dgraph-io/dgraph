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
	"io/ioutil"
	"os"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/dgraph-io/dgraph/commit"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/store"
)

func TestGetOrAssign(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)

	dir, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		t.Error(err)
		return
	}
	defer os.RemoveAll(dir)
	ps := new(store.Store)
	ps.Init(dir)
	clog := commit.NewLogger(dir, "mutations", 50<<20)
	clog.Init()
	defer clog.Close()

	posting.Init(ps, clog)

	var u1, u2 uint64
	{
		uid, err := GetOrAssign("externalid0")
		if err != nil {
			t.Error(err)
		}
		t.Logf("Found uid: [%x]", uid)
		u1 = uid
	}

	{
		uid, err := GetOrAssign("externalid1")
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
		uid, err := GetOrAssign("externalid0")
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
		xid, err := ExternalId(u1)
		if err != nil {
			t.Error(err)
		}
		if xid != "externalid0" {
			t.Errorf("Expected externalid0. Found: [%q]", xid)
		}
	}
	return
	{
		xid, err := ExternalId(u2)
		if err != nil {
			t.Error(err)
		}
		if xid != "externalid1" {
			t.Errorf("Expected externalid1. Found: [%q]", xid)
		}
	}
}
