// Copyright 2019 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package database

import (
	"bytes"
	"testing"
)

type memData struct {
	input    string
	expected string
}

func testData() []memData {
	tests := []memData{
		{"camel", "camel"},
		{"walrus", "walrus"},
		{"296204", "296204"},
		{"\x00123\x00", "\x00123\x00"},
	}
	return tests
}

func TestMemoryDB_PutGet(t *testing.T) {
	memDB := NewMemDatabase()
	testPutGet(memDB, t)
	testHasGet(memDB, t)
	testDelGet(memDB, t)
}

func testPutGet(db *MemDatabase, t *testing.T) {
	tests := testData()

	for _, v := range tests {
		err := db.Put([]byte(v.input), []byte(v.input))
		if err != nil {
			t.Fatalf("put failed: %v", err)
		}
	}
	for _, v := range tests {
		data, err := db.Get([]byte(v.input))
		if err != nil {
			t.Fatalf("get failed: %v", err)
		}
		if !bytes.Equal(data, []byte(v.expected)) {
			t.Fatalf("get returned wrong result, got %q expected %q", string(data), v.expected)
		}
	}
}

func testHasGet(db *MemDatabase, t *testing.T) {
	tests := testData()

	for _, v := range tests {
		exists, err := db.Has([]byte(v.input))
		if err != nil {
			t.Fatalf("has operation failed: %v", err)
		}
		if !exists {
			t.Fatalf("has operation returned wrong result, got %t expected %t", exists, true)
		}
	}
	k := db.Keys()
	if len(k) != 4 {
		t.Fatalf("failed to retrieve keys, expected %v, got %v", 4, len(k[0]))
	}
}

func testDelGet(db *MemDatabase, t *testing.T) {
	tests := testData()

	for _, v := range tests {
		err := db.Put([]byte(v.input), []byte(v.input))
		if err != nil {
			t.Fatalf("put failed: %v", err)
		}
	}

	for _, v := range tests {
		err := db.Del([]byte(v.input))
		if err != nil {
			t.Fatalf("delete %q failed: %v", v.input, err)
		}
	}

	for _, v := range tests {
		d, _ := db.Get([]byte(v.input))
		if len(d) > 1 {
			t.Fatalf("failed to delete value %q", v.input)
		}
	}
}
