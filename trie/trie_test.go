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

func TestCommonPrefix(t *testing.T) {
	for _, test := range commonPrefixTests {
		output := lenCommonPrefix(test.a, test.b)
		if output != test.output {
			t.Errorf("Fail: got %d expected %d", output, test.output)
		}
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
		buf := make([]byte, r.Intn(32))
		r.Read(buf)
		rt[i].key = buf

		buf = make([]byte, r.Intn(32))
		r.Read(buf)
		rt[i].value = buf
	}
	return rt
}

func TestPutAndGet(t *testing.T) {
	trie := newEmpty()

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
