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

package store

import (
	"io/ioutil"
	"os"
	"testing"
)

func TestGet(t *testing.T) {
	path, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		t.Error(err)
		t.Fail()
		return
	}
	defer os.RemoveAll(path)

	var s Store
	s.Init(path)
	if err := s.SetOne("name", 1, []byte("neo")); err != nil {
		t.Error(err)
		t.Fail()
	}

	if val, err := s.Get("name", 1); err != nil {
		t.Error(err)
		t.Fail()
	} else if string(val) != "neo" {
		t.Errorf("Expected 'neo'. Found: %s", string(val))
	}

	if err := s.SetOne("name", 1, []byte("the one")); err != nil {
		t.Error(err)
		t.Fail()
	}

	if val, err := s.Get("name", 1); err != nil {
		t.Error(err)
		t.Fail()
	} else if string(val) != "the one" {
		t.Errorf("Expected 'the one'. Found: %s", string(val))
	}
}
