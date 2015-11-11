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
)

func TestHandleFile(t *testing.T) {
	dir, err := ioutil.TempDir("", "dgraph-log")
	if err != nil {
		t.Error(err)
		return
	}
	defer os.RemoveAll(dir)

	l := NewLogger(dir, "dgraph", 50<<20)
	ts := time.Now().UnixNano()
	for i := 0; i < 10; i++ {
		ts += 1
		fp := filepath.Join(dir, l.filepath(ts))
		if err := ioutil.WriteFile(fp, []byte("test calling"),
			os.ModeAppend); err != nil {
			t.Error(err)
			return
		}
	}
	l.Init()
}
