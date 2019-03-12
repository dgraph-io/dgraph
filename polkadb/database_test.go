package polkadb

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"

	"fmt"
)

type data struct {
	input    string
	expected string
}

func newTestBadgerDB() (*BadgerDB, func()) {
	dir, err := ioutil.TempDir(os.TempDir(), "badger-test")
	if err != nil {
		panic("failed to create test file: " + err.Error())
	}
	db, err := NewBadgerDB(dir)
	if err != nil {
		panic("failed to create test database: " + err.Error())
	}
	return db, func() {
		db.Close()
		if err := os.RemoveAll(dir); err != nil {
			fmt.Println("removal of temp directory badger-test failed")
		}
	}
}

func testSetup() []data {
	tests := []data{
		{"camel", "camel"},
		{"walrus", "walrus"},
		{"296204", "296204"},
		{"\x00123\x00", "\x00123\x00"},
	}
	return tests
}

func TestBadgerDB_PutGetDel(t *testing.T) {
	db, remove := newTestBadgerDB()
	defer remove()
	testPutGetter(db, t)
	testHasGetter(db, t)
	testUpdateGetter(db, t)
	testDelGetter(db, t)
}

func testPutGetter(db Database, t *testing.T) {
	tests := testSetup()

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

func testHasGetter(db Database, t *testing.T) {
	tests := testSetup()

	for _, v := range tests {
		exists, err := db.Has([]byte(v.input))
		if err != nil {
			t.Fatalf("has operation failed: %v", err)
		}
		if !exists {
			t.Fatalf("has operation returned wrong result, got %t expected %t", exists, true)
		}
	}
}

func testUpdateGetter(db Database, t *testing.T) {
	tests := testSetup()

	for _, v := range tests {
		err := db.Put([]byte(v.input), []byte("?"))
		if err != nil {
			t.Fatalf("put override failed: %v", err)
		}
	}

	for _, v := range tests {
		data, err := db.Get([]byte(v.input))
		if err != nil {
			t.Fatalf("get failed: %v", err)
		}
		if !bytes.Equal(data, []byte("?")) {
			t.Fatalf("get returned wrong result, got %q expected ?", string(data))
		}
	}
}

func testDelGetter(db Database, t *testing.T) {
	tests := testSetup()

	for _, v := range tests {
		err := db.Del([]byte(v.input))
		if err != nil {
			t.Fatalf("delete %q failed: %v", v.input, err)
		}
	}

	for _, v := range tests {
		d, err := db.Get([]byte(v.input))
		if err != nil {
			t.Fatalf("got deleted value %q failed: %v", v.input, err)
		}
		if len(d) > 1 {
			t.Fatalf("failed to delete value %q", v.input)
		}
	}
}
