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
	"io/ioutil"
	"os"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

func TestGetPredicate(t *testing.T) {
	attr := "friends"
	uid, err := x.EncodeUint64(12345)
	if err != nil {
		t.Error(err)
		return
	}
	buf := bytes.NewBufferString(attr)
	_, err = buf.WriteRune('|')
	if err != nil {
		t.Error(err)
		return
	}
	_, err = buf.Write(uid)
	if err != nil {
		t.Error(err)
		return
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
	ps1, err := store.NewStore(dir1)
	if err != nil {
		t.Error(err)
		return
	}
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
