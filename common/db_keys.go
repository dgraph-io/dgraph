package common

//nolint
var (
	// LatestHeaderHashKey is the db location the hash of the latest block header.
	LatestHeaderHashKey = []byte("latest_hash")
	// LatestStorageHashKey is the db location of the hash of the latest storage trie.
	LatestStorageHashKey = []byte("latest_storage_hash")
	// GenesisDataKey is the db location of the genesis data.
	GenesisDataKey = []byte("genesis_data")
)
