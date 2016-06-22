/*
 * Copyright 2016 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package cluster

import (
	"bytes"
	"encoding/binary"
	"io/ioutil"
	"os"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/store"
)

func TestgetPredicate(t *testing.T) {
	attr := "friends"
	var uid uint64 = 12345

	buf := bytes.NewBufferString(attr)
	if err := binary.Write(buf, binary.LittleEndian, uid); err != nil {
		t.Fatalf("Error while creating key with attr: %v uid: %v\n", attr, uid)
	}

	str := getPredicate(buf.Bytes())
	if str != "friends" {
		t.Error("Wrong predicate obtained")
	}
}

func TestGetPredicateList(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)
	dir1, err := ioutil.TempDir("", "dir_")
	if err != nil {
		t.Error(err)
		return
	}
	defer os.RemoveAll(dir1)
	ps1 := new(store.Store)
	ps1.Init(dir1)
	defer ps1.Close()

	k1 := posting.Key(1000, "friend")
	k2 := posting.Key(1010, "friend")
	k3 := posting.Key(1020, "friend")
	k4 := posting.Key(1030, "follow")
	k5 := posting.Key(1040, "follow")
	ps1.SetOne(k1, []byte("alice"))
	ps1.SetOne(k2, []byte("bob"))
	ps1.SetOne(k3, []byte("ram"))
	ps1.SetOne(k4, []byte("ash"))
	ps1.SetOne(k5, []byte("mallory"))

	list := GetPredicateList(ps1)

	if len(list) != 2 {
		t.Errorf("Predicate List incorrect %v", len(list))
	}
}
