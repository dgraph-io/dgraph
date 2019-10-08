package polkadb

// BlockDB stores block's in an underlying Database
type BlockDB struct {
	Db Database
}

// NewBlockDB instantiates a badgerDB instance for storing relevant BlockData
func NewBlockDB(dataDir string) (*BlockDB, error) {
	db, err := NewBadgerDB(dataDir)
	if err != nil {
		return nil, err
	}

	return &BlockDB{
		db,
	}, nil
}
