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

package commit

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
)

func TestHandleFile(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)

	dir, err := ioutil.TempDir("", "dgraph-log")
	if err != nil {
		t.Error(err)
		return
	}
	defer os.RemoveAll(dir)

	l := NewLogger(dir, "dgraph", 50<<20)
	ts := time.Now().UnixNano()
	for i := 0; i < 10; i++ {
		fp := filepath.Join(dir, l.filepath(ts+int64(i)))
		if err := ioutil.WriteFile(fp, []byte("test calling"),
			os.ModeAppend); err != nil {
			t.Error(err)
			return
		}
	}
	l.Init()
	for i, lf := range l.list {
		exp := ts + int64(i)
		if lf.endTs != exp {
			t.Errorf("Expected %v. Got: %v", exp, lf.endTs)
		}
	}
}

func TestAddLog(t *testing.T) {
	dir, err := ioutil.TempDir("", "dgraph-log")
	if err != nil {
		t.Error(err)
		return
	}
	defer os.RemoveAll(dir)

	l := NewLogger(dir, "dgraph", 50<<20)
	l.Init()

	ts := time.Now().UnixNano()
	for i := 0; i < 10; i++ {
		curts := ts + int64(i)
		if err := l.AddLog(curts, 0, []byte("hey")); err != nil {
			t.Error(err)
			return
		}
	}

	glog.Debugf("Test curfile path: %v", l.curFile.Name())
	last, err := lastTimestamp(l.curFile.Name())
	if err != nil {
		t.Error(err)
	}
	if last != ts+9 {
		t.Errorf("Expected %v. Got: %v\n", ts+9, last)
	}
}
