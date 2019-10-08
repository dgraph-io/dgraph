package polkadb

// StateDB stores trie structure in an underlying Database
type StateDB struct {
	Db Database
}

// NewStateDB instantiates badgerDB instance for storing trie structure
func NewStateDB(dataDir string) (*StateDB, error) {
	db, err := NewBadgerDB(dataDir)
	if err != nil {
		return nil, err
	}

	return &StateDB{
		db,
	}, nil
}
