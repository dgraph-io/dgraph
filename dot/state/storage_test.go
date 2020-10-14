package state

import (
	"math/big"
	"testing"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/trie"

	database "github.com/ChainSafe/chaindb"
	"github.com/stretchr/testify/require"
)

func newTestStorageState(t *testing.T) *StorageState {
	db := database.NewMemDatabase()
	bs := newTestBlockState(t, testGenesisHeader)

	s, err := NewStorageState(db, bs, trie.NewEmptyTrie())
	require.NoError(t, err)
	return s
}

func TestStorage_LoadCodeHash(t *testing.T) {
	storage := newTestStorageState(t)
	testCode := []byte("asdf")

	err := storage.setStorage(nil, codeKey, testCode)
	require.NoError(t, err)

	resCode, err := storage.LoadCode(nil)
	require.NoError(t, err)
	require.Equal(t, testCode, resCode)

	resHash, err := storage.LoadCodeHash(nil)
	require.NoError(t, err)

	expectedHash, err := common.HexToHash("0xb91349ff7c99c3ae3379dd49c2f3208e202c95c0aac5f97bb24ded899e9a2e83")
	require.NoError(t, err)
	require.Equal(t, expectedHash, resHash)
}

func TestStorage_SetAndGetBalance(t *testing.T) {
	storage := newTestStorageState(t)

	key := [32]byte{1, 2, 3, 4, 5, 6, 7}
	bal := uint64(99)

	err := storage.setBalance(nil, key, bal)
	require.NoError(t, err)

	res, err := storage.GetBalance(nil, key)
	require.NoError(t, err)
	require.Equal(t, bal, res)
}

func TestStorage_StoreAndLoadTrie(t *testing.T) {
	storage := newTestStorageState(t)
	ts, err := storage.TrieState(&trie.EmptyHash)
	require.NoError(t, err)

	root, err := ts.Root()
	require.NoError(t, err)
	err = storage.StoreTrie(root, ts)
	require.NoError(t, err)

	trie, err := storage.LoadFromDB(root)
	require.NoError(t, err)
	ts2, err := NewTrieState(storage.baseDB, trie)
	require.NoError(t, err)
	require.Equal(t, ts, ts2)
}

func TestStorage_GetStorageByBlockHash(t *testing.T) {
	storage := newTestStorageState(t)
	ts, err := storage.TrieState(&trie.EmptyHash)
	require.NoError(t, err)

	key := []byte("testkey")
	value := []byte("testvalue")
	err = ts.Set(key, value)
	require.NoError(t, err)

	root, err := ts.Root()
	require.NoError(t, err)
	err = storage.StoreTrie(root, ts)
	require.NoError(t, err)

	block := &types.Block{
		Header: &types.Header{
			ParentHash: testGenesisHeader.Hash(),
			Number:     big.NewInt(1),
			StateRoot:  root,
		},
		Body: types.NewBody([]byte{}),
	}
	err = storage.blockState.AddBlock(block)
	require.NoError(t, err)

	res, err := storage.GetStorageByBlockHash(block.Header.Hash(), key)
	require.NoError(t, err)
	require.Equal(t, value, res)
}
