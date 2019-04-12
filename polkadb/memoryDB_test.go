package polkadb

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
