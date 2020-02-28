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

package state

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"sync"

	"github.com/ChainSafe/gossamer/dot/core/types"
	babetypes "github.com/ChainSafe/gossamer/lib/babe/types"
	"github.com/ChainSafe/gossamer/lib/blocktree"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/common/optional"
	"github.com/ChainSafe/gossamer/lib/database"
)

var blockPrefix = []byte("block")

// BlockDB stores block's in an underlying Database
type BlockDB struct {
	db database.Database
}

// Put appends `block` to the key and sets the key-value pair in the db
func (blockDB *BlockDB) Put(key, value []byte) error {
	key = append(blockPrefix, key...)
	return blockDB.db.Put(key, value)
}

// Get appends `block` to the key and retrieves the value from the db
func (blockDB *BlockDB) Get(key []byte) ([]byte, error) {
	key = append(blockPrefix, key...)
	return blockDB.db.Get(key)
}

// BlockState defines fields for manipulating the state of blocks, such as BlockTree, BlockDB and Header
type BlockState struct {
	bt           *blocktree.BlockTree
	db           *BlockDB
	latestHeader *types.Header
	lock         sync.RWMutex
}

// NewBlockDB instantiates a badgerDB instance for storing relevant BlockData
func NewBlockDB(db database.Database) *BlockDB {
	return &BlockDB{
		db,
	}
}

// NewBlockState will create a new BlockState backed by the database located at dataDir
func NewBlockState(db database.Database, latestHash common.Hash) (*BlockState, error) {
	if db == nil {
		return nil, fmt.Errorf("cannot have nil database")
	}

	bs := &BlockState{
		bt: &blocktree.BlockTree{},
		db: NewBlockDB(db),
	}

	latestHeader, err := bs.GetHeader(latestHash)
	if err != nil {
		return bs, fmt.Errorf("NewBlockState latestBlock err: %s", err)
	}

	bs.latestHeader = latestHeader
	return bs, nil
}

// NewBlockStateFromGenesis initializes a BlockState from a genesis header, saving it to the database located at dataDir
func NewBlockStateFromGenesis(db database.Database, header *types.Header) (*BlockState, error) {
	bs := &BlockState{
		bt: &blocktree.BlockTree{},
		db: NewBlockDB(db),
	}

	err := bs.SetHeader(header)
	if err != nil {
		return nil, err
	}

	bs.latestHeader = header
	return bs, nil
}

var (
	// Data prefixes
	headerPrefix     = []byte("hdr") // headerPrefix + hash -> header
	babeHeaderPrefix = []byte("hba") // babeHeaderPrefix || epoch || slot -> babeHeader
	blockDataPrefix  = []byte("bld") // blockDataPrefix + hash -> blockData
	headerHashPrefix = []byte("hsh") // headerHashPrefix + encodedBlockNum -> hash
)

// encodeBlockNumber encodes a block number as big endian uint64
func encodeBlockNumber(number uint64) []byte {
	enc := make([]byte, 8) // encoding results in 8 bytes
	binary.BigEndian.PutUint64(enc, number)
	return enc
}

// headerKey = headerPrefix + hash
func headerKey(hash common.Hash) []byte {
	return append(headerPrefix, hash.ToBytes()...)
}

// headerHashKey = headerHashPrefix + num (uint64 big endian)
func headerHashKey(number uint64) []byte {
	return append(headerHashPrefix, encodeBlockNumber(number)...)
}

// blockDataKey = blockDataPrefix + hash
func blockDataKey(hash common.Hash) []byte {
	return append(blockDataPrefix, hash.ToBytes()...)
}

// GetHeader returns a BlockHeader for a given hash
func (bs *BlockState) GetHeader(hash common.Hash) (*types.Header, error) {
	result := new(types.Header)

	data, err := bs.db.Get(headerKey(hash))
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, result)
	if reflect.DeepEqual(result, new(types.Header)) {
		return nil, fmt.Errorf("header does not exist")
	}

	result.Hash()
	return result, err
}

// GetBlockData returns a BlockData for a given hash
func (bs *BlockState) GetBlockData(hash common.Hash) (*types.BlockData, error) {
	result := new(types.BlockData)

	data, err := bs.db.Get(blockDataKey(hash))
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, result)
	if err != nil {
		return result, err
	}

	if result.Header == nil {
		result.Header = optional.NewHeader(false, nil)
	}

	if result.Body == nil {
		result.Body = optional.NewBody(false, nil)
	}

	if result.Receipt == nil {
		result.Receipt = optional.NewBytes(false, nil)
	}

	if result.MessageQueue == nil {
		result.MessageQueue = optional.NewBytes(false, nil)
	}

	if result.Justification == nil {
		result.Justification = optional.NewBytes(false, nil)
	}

	return result, nil
}

// LatestHeader returns the latest block available on BlockState
func (bs *BlockState) LatestHeader() *types.Header {
	return bs.latestHeader.DeepCopy()
}

// GetBlockByHash returns a block for a given hash
func (bs *BlockState) GetBlockByHash(hash common.Hash) (*types.Block, error) {
	header, err := bs.GetHeader(hash)
	if err != nil {
		return nil, err
	}

	blockData, err := bs.GetBlockData(hash)
	if err != nil {
		return nil, err
	}

	body, err := types.NewBodyFromOptional(blockData.Body)
	if err != nil {
		return nil, err
	}
	return &types.Block{Header: header, Body: body}, nil
}

// GetBlockByNumber returns a block for a given blockNumber
func (bs *BlockState) GetBlockByNumber(blockNumber *big.Int) (*types.Block, error) {
	// First retrieve the block hash in a byte array based on the block number from the database
	byteHash, err := bs.db.Get(headerHashKey(blockNumber.Uint64()))
	if err != nil {
		return nil, err
	}

	// Then find the block based on the hash
	hash := common.NewHash(byteHash)
	block, err := bs.GetBlockByHash(hash)
	if err != nil {
		return nil, err
	}

	return block, nil
}

// SetHeader will set the header into DB
func (bs *BlockState) SetHeader(header *types.Header) error {
	bs.lock.Lock()
	defer bs.lock.Unlock()

	hash := header.Hash()

	// Write the encoded header
	bh, err := json.Marshal(header)
	if err != nil {
		return err
	}

	err = bs.db.Put(headerKey(hash), bh)
	if err != nil {
		return err
	}

	// Add a mapping of [blocknumber : hash] for retrieving the block by number
	err = bs.db.Put(headerHashKey(header.Number.Uint64()), header.Hash().ToBytes())
	return err
}

// SetBlock will set a block using BlockState SetBlockData method
func (bs *BlockState) SetBlock(block *types.Block) error {
	blockData := &types.BlockData{
		Hash:   block.Header.Hash(),
		Header: block.Header.AsOptional(),
		Body:   block.Body.AsOptional(),
	}
	return bs.SetBlockData(blockData)
}

// SetBlockData will set the block data using given hash and blockData into DB
func (bs *BlockState) SetBlockData(blockData *types.BlockData) error {
	bs.lock.Lock()
	defer bs.lock.Unlock()

	// Write the encoded header
	bh, err := json.Marshal(blockData)
	if err != nil {
		return err
	}

	err = bs.db.Put(blockDataKey(blockData.Hash), bh)
	return err
}

// AddBlock will set the latestBlock in BlockState DB
func (bs *BlockState) AddBlock(newBlock *types.Block) error {
	// Set the latest block
	// If latestHeader is nil OR the new block number is greater than current block number
	if bs.latestHeader == nil || (newBlock.Header.Number != nil && newBlock.Header.Number.Cmp(bs.latestHeader.Number) == 1) {
		bs.latestHeader = newBlock.Header.DeepCopy()
	}

	// Add the blockHeader to the DB
	err := bs.SetHeader(bs.latestHeader)
	if err != nil {
		return err
	}

	hash := newBlock.Header.Hash()
	err = bs.setLatestHeaderKey(hash)
	if err != nil {
		return err
	}

	// Create BlockData
	bd := &types.BlockData{
		Hash:   hash,
		Header: newBlock.Header.AsOptional(),
		Body:   newBlock.Body.AsOptional(),
	}
	err = bs.SetBlockData(bd)
	return err
}

func (bs *BlockState) setLatestHeaderKey(hash common.Hash) error {
	return bs.db.db.Put(common.LatestHeaderHashKey, hash[:])
}

// babeHeaderKey = babeHeaderPrefix || epoch || slice
func babeHeaderKey(epoch uint64, slot uint64) []byte {
	epochBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(epochBytes, epoch)
	sliceBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(sliceBytes, slot)
	combined := append(epochBytes, sliceBytes...)
	return append(babeHeaderPrefix, combined...)
}

// GetBabeHeader retrieves a BabeHeader from the database
func (bs *BlockState) GetBabeHeader(epoch uint64, slot uint64) (*babetypes.BabeHeader, error) {
	result := new(babetypes.BabeHeader)

	data, err := bs.db.Get(babeHeaderKey(epoch, slot))
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, result)

	return result, err
}

// SetBabeHeader sets a BabeHeader in the database
func (bs *BlockState) SetBabeHeader(epoch uint64, slot uint64, bh *babetypes.BabeHeader) error {
	// Write the encoded header
	enc, err := json.Marshal(bh)
	if err != nil {
		return err
	}

	err = bs.db.Put(babeHeaderKey(epoch, slot), enc)
	return err
}
