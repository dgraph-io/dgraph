package rawdb

import (
	"encoding/json"

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/core/types"
	"github.com/ChainSafe/gossamer/polkadb"
)

// check checks to see if there an error if so writes err + message to terminal
func check(e error) {
	if e != nil {
		panic(e)
	}
}

// SetHeader stores a block header into the KV-store; key is headerPrefix + hash
func SetHeader(db polkadb.Writer, header *types.BlockHeaderWithHash) {
	hash := header.Hash

	// Write the encoded header
	bh, err := json.Marshal(header)
	check(err)

	err = db.Put(headerKey(hash), bh)
	check(err)
}

// GetHeader retrieves block header from KV-store using headerKey
func GetHeader(db polkadb.Reader, hash common.Hash) types.BlockHeaderWithHash {
	var result types.BlockHeaderWithHash
	get(db, headerKey(hash), &result)
	return result
}

// SetBlockData writes blockData to KV-store; key is blockDataPrefix + hash
func SetBlockData(db polkadb.Writer, blockData *types.BlockData) {
	hash := blockData.Hash

	// Write the encoded header
	bh, err := json.Marshal(blockData)
	check(err)

	err = db.Put(blockDataKey(hash), bh)
	check(err)
}

// GetBlockData retrieves blockData from KV-store using blockDataKey
func GetBlockData(db polkadb.Reader, hash common.Hash) types.BlockData {
	var result types.BlockData
	get(db, blockDataKey(hash), &result)
	return result
}

// get is a helper function for retrieving a value from KV-store and unmarshaling
// into the provided type out
func get(db polkadb.Reader, hash []byte, out interface{}) {
	data, err := db.Get(hash)
	check(err)

	err = json.Unmarshal(data, &out)
	check(err)
}
