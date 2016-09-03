/*
 * Copyright 2016 Dgraph Labs, Inc.
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

package indexer

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
)

func arrayCompare(a []string, b []string) error {
	if len(a) != len(b) {
		return fmt.Errorf("Size mismatch %d vs %d", len(a), len(b))
	}
	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return fmt.Errorf("Element mismatch at index %d", i)
		}
	}
	return nil
}

func checkQuery(i Indexer, pred, val string, expected []string) error {
	results, err := i.Query(pred, val)
	if err != nil {
		return err
	}
	return arrayCompare(results, expected)
}

func testBasic(i Indexer, t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	err = i.Create(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer i.Close()

	i.Insert("p1", "k1", "v1")
	if err := checkQuery(i, "p2", "v1", []string{}); err != nil {
		t.Fatal(err)
	}
	if err := checkQuery(i, "p1", "v2", []string{}); err != nil {
		t.Fatal(err)
	}
	if err := checkQuery(i, "p1", "v1", []string{"k1"}); err != nil {
		t.Fatal(err)
	}

	if err := i.Insert("p2", "k2", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := checkQuery(i, "p1", "v1", []string{"k1"}); err != nil {
		t.Fatal(err)
	}

	if err := i.Insert("p1", "k2", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := checkQuery(i, "p1", "v1", []string{"k1", "k2"}); err != nil {
		t.Fatal(err)
	}

	if err := i.Insert("p1", "k0", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := checkQuery(i, "p1", "v1", []string{"k0", "k1", "k2"}); err != nil {
		t.Fatal(err)
	}

	// Delete something that is not present.
	if err := i.Remove("p2", "k2"); err != nil {
		t.Fatal(err)
	}
	if err := checkQuery(i, "p1", "v1", []string{"k0", "k1", "k2"}); err != nil {
		t.Fatal(err)
	}

	if err := i.Remove("p1", "k1"); err != nil {
		t.Fatal(err)
	}
	if err := checkQuery(i, "p1", "v1", []string{"k0", "k2"}); err != nil {
		t.Fatal(err)
	}
}

func testBatch(i Indexer, t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	err = i.Create(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer i.Close()

	b, err := i.NewBatch()
	if err != nil {
		t.Fatal(err)
	}
	err = b.Insert("p1", "k1", "v1")
	if err != nil {
		t.Fatal(err)
	}
	err = b.Insert("p2", "k1", "v1")
	if err != nil {
		t.Fatal(err)
	}
	err = b.Insert("p1", "k2", "v1")
	if err != nil {
		t.Fatal(err)
	}
	err = b.Insert("p1", "k0", "v1")
	if err != nil {
		t.Fatal(err)
	}
	err = b.Remove("p1", "k1")
	if err != nil {
		t.Fatal(err)
	}

	err = i.Batch(b)
	if err != nil {
		t.Fatal(err)
	}

	err = checkQuery(i, "p1", "v1", []string{"k0", "k2"})
	if err != nil {
		t.Fatal(err)
	}
}

func testOverwrite(i Indexer, t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	err = i.Create(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer i.Close()

	i.Insert("p1", "k1", "v1")
	if err := checkQuery(i, "p1", "v1", []string{"k1"}); err != nil {
		t.Fatal(err)
	}

	if err := i.Insert("p1", "k1", "v2"); err != nil {
		t.Fatal(err)
	}
	if err := checkQuery(i, "p1", "v1", []string{}); err != nil {
		t.Fatal(err)
	}
	if err := checkQuery(i, "p1", "v2", []string{"k1"}); err != nil {
		t.Fatal(err)
	}

	// Let's add new keys.
	if err := i.Insert("p1", "k0", "v2"); err != nil {
		t.Fatal(err)
	}
	if err := i.Insert("p1", "k2", "v2"); err != nil {
		t.Fatal(err)
	}
	if err := checkQuery(i, "p1", "v2", []string{"k0", "k1", "k2"}); err != nil {
		t.Fatal(err)
	}

	// Overwrite values of all keys.
	if err := i.Insert("p1", "k0", "v3"); err != nil {
		t.Fatal(err)
	}
	if err := i.Insert("p1", "k2", "v3"); err != nil {
		t.Fatal(err)
	}
	if err := i.Insert("p1", "k1", "v3"); err != nil {
		t.Fatal(err)
	}
	if err := checkQuery(i, "p1", "v2", []string{}); err != nil {
		t.Fatal(err)
	}
	if err := checkQuery(i, "p1", "v3", []string{"k0", "k1", "k2"}); err != nil {
		t.Fatal(err)
	}
}

// TestAll tests the given indexer.
func TestAll(f func() Indexer, t *testing.T) {
	testBasic(f(), t)
	testBatch(f(), t)
	testOverwrite(f(), t)
}
