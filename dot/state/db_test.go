package state

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/trie"

	database "github.com/ChainSafe/chaindb"
)

func TestTrie_StoreAndLoadFromDB(t *testing.T) {
	db := database.NewMemDatabase()
	tt := trie.NewEmptyTrie()

	rt := trie.GenerateRandomTests(t, 1000)
	var val []byte
	for _, test := range rt {
		err := tt.Put(test.Key(), test.Value())
		if err != nil {
			t.Errorf("Fail to put with key %x and value %x: %s", test.Key(), test.Value(), err.Error())
		}

		val, err = tt.Get(test.Key())
		if err != nil {
			t.Errorf("Fail to get key %x: %s", test.Key(), err.Error())
		} else if !bytes.Equal(val, test.Value()) {
			t.Errorf("Fail to get key %x with value %x: got %x", test.Key(), test.Value(), val)
		}
	}

	err := StoreTrie(db, tt)
	if err != nil {
		t.Fatalf("Fail: could not write trie to DB: %s", err)
	}

	encroot, err := tt.Hash()
	if err != nil {
		t.Fatal(err)
	}

	expected := trie.NewTrie(tt.RootNode())

	tt = trie.NewEmptyTrie()
	err = LoadTrie(db, tt, encroot)
	if err != nil {
		t.Errorf("Fail: could not load trie from DB: %s", err)
	}

	if strings.Compare(expected.String(), tt.String()) != 0 {
		t.Errorf("Fail: got\n %s expected\n %s", expected.String(), tt.String())
	}

	if !reflect.DeepEqual(expected.RootNode(), tt.RootNode()) {
		t.Errorf("Fail: got\n %s expected\n %s", expected.RootNode(), tt.RootNode())
	}
}

type test struct {
	key   []byte
	value []byte
}

func TestStoreAndLoadLatestStorageHash(t *testing.T) {
	db := database.NewMemDatabase()
	tt := trie.NewEmptyTrie()

	tests := []test{
		{key: []byte{0x01, 0x35}, value: []byte("pen")},
		{key: []byte{0x01, 0x35, 0x79}, value: []byte("penguin")},
		{key: []byte{0x01, 0x35, 0x7}, value: []byte("g")},
		{key: []byte{0xf2}, value: []byte("feather")},
		{key: []byte{0xf2, 0x3}, value: []byte("f")},
		{key: []byte{0x09, 0xd3}, value: []byte("noot")},
		{key: []byte{0x07}, value: []byte("ramen")},
		{key: []byte{0}, value: nil},
	}

	for _, test := range tests {
		err := tt.Put(test.key, test.value)
		if err != nil {
			t.Fatal(err)
		}
	}

	err := StoreLatestStorageHash(db, tt)
	if err != nil {
		t.Fatal(err)
	}

	hash, err := LoadLatestStorageHash(db)
	if err != nil {
		t.Fatal(err)
	}

	expected, err := tt.Hash()
	if err != nil {
		t.Fatal(err)
	}

	if hash != expected {
		t.Fatalf("Fail: got %x expected %x", hash, expected)
	}
}

func TestStoreAndLoadGenesisData(t *testing.T) {
	db := database.NewMemDatabase()

	bootnodes := common.StringArrayToBytes([]string{
		"/ip4/127.0.0.1/tcp/7001/p2p/12D3KooWHHzSeKaY8xuZVzkLbKFfvNgPPeKhFBGrMbNzbm5akpqu",
		"/ip4/127.0.0.1/tcp/7001/p2p/12D3KooWHHzSeKaY8xuZVzkLbKFfvNgPPeKhFBGrMbNzbm5akpqu",
	})

	expected := &genesis.Data{
		Name:       "gossamer",
		ID:         "gossamer",
		Bootnodes:  bootnodes,
		ProtocolID: "/gossamer/test/0",
	}

	err := StoreGenesisData(db, expected)
	if err != nil {
		t.Fatal(err)
	}

	gen, err := LoadGenesisData(db)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(gen, expected) {
		t.Fatalf("Fail: got %v expected %v", gen, expected)
	}
}

func TestStoreAndLoadBestBlockHash(t *testing.T) {
	db := database.NewMemDatabase()
	hash, _ := common.HexToHash("0x3f5a19b9e9507e05276216f3877bb289e47885f8184010c65d0e41580d3663cc")

	err := StoreBestBlockHash(db, hash)
	if err != nil {
		t.Fatal(err)
	}

	res, err := LoadBestBlockHash(db)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(res, hash) {
		t.Fatalf("Fail: got %x expected %x", res, hash)
	}
}
