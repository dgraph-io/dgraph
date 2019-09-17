package polkadb

import (
	log "github.com/ChainSafe/log15"
)

// StateDB stores trie structure in an underlying Database
type StateDB struct {
	Db Database
}

// NewStateDB instantiates badgerDB instance for storing trie structure
func NewStateDB(dataDir string) (*StateDB, error) {
	db, err := NewBadgerDB(dataDir)
	if err != nil {
		log.Crit("error instantiating StateDB", "error", err)
		return nil, err
	}

	return &StateDB{
		db,
	}, nil
}
