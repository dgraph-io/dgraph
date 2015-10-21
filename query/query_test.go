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

package query

import (
	"io/ioutil"
	"math"
	"os"
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/manishrjain/dgraph/posting"
	"github.com/manishrjain/dgraph/query/result"
	"github.com/manishrjain/dgraph/store"
	"github.com/manishrjain/dgraph/x"
)

func setErr(err *error, nerr error) {
	if err != nil {
		return
	}
	*err = nerr
}

func populateList(key []byte) error {
	pl := posting.Get(key)

	t := x.Triple{
		ValueId:   9,
		Source:    "query_test",
		Timestamp: time.Now(),
	}
	var err error
	setErr(&err, pl.AddMutation(t, posting.Set))

	t.ValueId = 19
	setErr(&err, pl.AddMutation(t, posting.Set))

	t.ValueId = 29
	setErr(&err, pl.AddMutation(t, posting.Set))

	t.Value = "abracadabra"
	setErr(&err, pl.AddMutation(t, posting.Set))

	return err
}

func NewStore(t *testing.T) string {
	path, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		t.Error(err)
		t.Fail()
		return ""
	}
	return path
}

func TestRun(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)

	pdir := NewStore(t)
	defer os.RemoveAll(pdir)
	ps := new(store.Store)
	ps.Init(pdir)

	mdir := NewStore(t)
	defer os.RemoveAll(mdir)
	ms := new(store.Store)
	ms.Init(mdir)
	posting.Init(ps, ms)

	key := posting.Key(11, "testing")
	if err := populateList(key); err != nil {
		t.Error(err)
	}
	key = posting.Key(9, "name")

	m := Message{Id: 11}
	ma := Mattr{Attr: "testing"}
	m.Attrs = append(m.Attrs, ma)

	if err := Run(&m); err != nil {
		t.Error(err)
	}
	ma = m.Attrs[0]
	uids := result.GetRootAsUids(ma.ResultUids, 0)
	if uids.UidLength() != 3 {
		t.Errorf("Expected 3. Got: %v", uids.UidLength())
	}
	var v uint64
	v = 9
	for i := 0; i < uids.UidLength(); i++ {
		if uids.Uid(i) == math.MaxUint64 {
			t.Error("Value posting encountered at index:", i)
		}
		if v != uids.Uid(i) {
			t.Errorf("Expected: %v. Got: %v", v, uids.Uid(i))
		}
		v += 10
	}
	log.Debugf("ResultUid buffer size: %v", len(ma.ResultUids))

	var val string
	if err := posting.ParseValue(&val, ma.ResultValue); err != nil {
		t.Error(err)
	}
	if val != "abracadabra" {
		t.Errorf("Expected abracadabra. Got: [%q]", val)
	}
}
