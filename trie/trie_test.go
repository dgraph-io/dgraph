package trie

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/ChainSafe/gossamer/polkadb"
)

type commonPrefixTest struct {
	a, b   []byte
	output int
}

var commonPrefixTests = []commonPrefixTest{
	{a: []byte{}, b: []byte{}, output: 0},
	{a: []byte{0x00}, b: []byte{}, output: 0},
	{a: []byte{0x00}, b: []byte{0x00}, output: 1},
	{a: []byte{0x00}, b: []byte{0x00, 0x01}, output: 1},
	{a: []byte{0x01}, b: []byte{0x00, 0x01, 0x02}, output: 0},
	{a: []byte{0x00, 0x01, 0x02, 0x00}, b: []byte{0x00, 0x01, 0x02}, output: 3},
	{a: []byte{0x00, 0x01, 0x02, 0x00, 0xff}, b: []byte{0x00, 0x01, 0x02, 0x00}, output: 4},
	{a: []byte{0x00, 0x01, 0x02, 0x00, 0xff}, b: []byte{0x00, 0x01, 0x02, 0x00, 0xff, 0x00}, output: 5},
}

func TestCommonPrefix(t *testing.T) {
	for _, test := range commonPrefixTests {
		output := lenCommonPrefix(test.a, test.b)
		if output != test.output {
			t.Errorf("Fail: got %d expected %d", output, test.output)
		}
	}
}

func newEmpty() *Trie {
	db, _ := polkadb.NewMemDatabase()
	t := NewEmptyTrie(db)
	return t
}

func TestNewEmptyTrie(t *testing.T) {
	trie := newEmpty()
	if trie == nil {
		t.Error("did not initialize trie")
	}
}

func TestNewTrie(t *testing.T) {
	db, _ := polkadb.NewMemDatabase()
	trie := NewTrie(db, leaf([]byte{0}), [32]byte{})
	if trie == nil {
		t.Error("did not initialize trie")
	}
}

type randTest struct {
	key   []byte
	value []byte
}

func generateRandTest(size int) []randTest {
	rt := make([]randTest, size)
	r := *rand.New(rand.NewSource(rand.Int63()))
	for i := range rt {
		rt[i] = randTest{}
		buf := make([]byte, r.Intn(379)+1)
		r.Read(buf)
		rt[i].key = buf

		buf = make([]byte, r.Intn(128))
		r.Read(buf)
		rt[i].value = buf
	}
	return rt
}

func TestPutNilKey(t *testing.T) {
	trie := newEmpty()
	err := trie.Put(nil, []byte{17})
	if err == nil {
		t.Errorf("did not error when attempting to put nil key")
	}
}

func TestExtension(t *testing.T) {
	trie := newEmpty()

	key1 := []byte("pen")
	value1 := []byte("nep")
	key2 := []byte("penguin")
	value2 := []byte("niugnep")

	err := trie.Put(key1, value1)
	if err != nil {
		t.Errorf("Fail to put with key %x and value %x: %s", key1, value1, err.Error())
	}

	err = trie.Put(key2, value2)
	if err != nil {
		t.Errorf("Fail to put with key %x and value %x: %s", key2, value2, err.Error())
	}

	val, err := trie.Get(key1)
	if err != nil {
		t.Errorf("Fail to get key %x: %s", key1, err.Error())
	} else if !bytes.Equal(val, value1) {
		t.Errorf("Fail to get key %x with value %x: got %x", key1, value1, val)
	}

	val, err = trie.Get(key2)
	if err != nil {
		t.Errorf("Fail to get key %x: %s", key2, err.Error())
	} else if !bytes.Equal(val, value2) {
		t.Errorf("Fail to get key %x with value %x: got %x", key2, value2, val)
	}
}

func TestBranch(t *testing.T) {
	trie := newEmpty()

	key1 := []byte("noot0")
	value1 := []byte("spaghetti")
	key2 := []byte("noot1")
	value2 := []byte("gnocchi")
	key3 := []byte("noot2")
	value3 := []byte("ramen")

	err := trie.Put(key1, value1)
	if err != nil {
		t.Errorf("Fail to put with key %x and value %x: %s", key1, value1, err.Error())
	}

	err = trie.Put(key2, value2)
	if err != nil {
		t.Errorf("Fail to put with key %x and value %x: %s", key2, value2, err.Error())
	}

	err = trie.Put(key3, value3)
	if err != nil {
		t.Errorf("Fail to put with key %x and value %x: %s", key3, value3, err.Error())
	}

	val, err := trie.Get([]byte("noot"))
	if err != nil {
		t.Errorf("Fail to get key %x: %s", key1, err.Error())
	} else if !bytes.Equal(val, nil) {
		t.Errorf("Fail to get key %x with nil value: got %x", "noot", val)
	}

	val, err = trie.Get(key1)
	if err != nil {
		t.Errorf("Fail to get key %x: %s", key1, err.Error())
	} else if !bytes.Equal(val, value1) {
		t.Errorf("Fail to get key %x with value %x: got %x", key1, value1, val)
	}

	val, err = trie.Get(key2)
	if err != nil {
		t.Errorf("Fail to get key %x: %s", key2, err.Error())
	} else if !bytes.Equal(val, value2) {
		t.Errorf("Fail to get key %x with value %x: got %x", key2, value2, val)
	}

	val, err = trie.Get(key3)
	if err != nil {
		t.Errorf("Fail to get key %x: %s", key3, err.Error())
	} else if !bytes.Equal(val, value3) {
		t.Errorf("Fail to get key %x with value %x: got %x", key3, value3, val)
	}
}

func TestPutAndGet(t *testing.T) {
	trie := newEmpty()

	for i := 0; i < 20; i++ {
		rt := generateRandTest(100)
		for _, test := range rt {
			err := trie.Put(test.key, test.value)
			if err != nil {
				t.Errorf("Fail to put with key %x and value %x: %s", test.key, test.value, err.Error())
			}

			val, err := trie.Get(test.key)
			if err != nil {
				t.Errorf("Fail to get key %x: %s", test.key, err.Error())
			} else if !bytes.Equal(val, test.value) {
				t.Errorf("Fail to get key %x with value %x: got %x", test.key, test.value, val)
			}
		}
	}
}

func TestDelete(t *testing.T) {
	trie := newEmpty()

	rt := generateRandTest(100)
	for _, test := range rt {
		err := trie.Put(test.key, test.value)
		if err != nil {
			t.Errorf("Fail to put with key %x and value %x: %s", test.key, test.value, err.Error())
		}
	}

	for _, test := range rt {
		r := rand.Int() % 2
		switch r {
		case 0:
			err := trie.Delete(test.key)
			if err != nil {
				t.Errorf("Fail to delete key %x: %s", test.key, err.Error())
			}

			val, err := trie.Get(test.key)
			if err != nil {
				t.Errorf("Error when attempting to get deleted key %x: %s", test.key, err.Error())
			} else if val != nil {
				t.Errorf("Fail to delete key %x with value %x: got %x", test.key, test.value, val)
			}
		case 1:
			val, err := trie.Get(test.key)
			if err != nil {
				t.Errorf("Error when attempting to get key %x: %s", test.key, err.Error())
			} else if !bytes.Equal(test.value, val) {
				t.Errorf("Fail to get key %x with value %x: got %x", test.key, test.value, val)
			}
		}
	}
}
