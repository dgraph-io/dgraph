package trie

import (
	"github.com/ChainSafe/gossamer/polkadb"
	//"sync"
)

// Database is a wrapper around a polkadb
type Database struct {
	db polkadb.Database
	//lock sync.RWMutex
}
