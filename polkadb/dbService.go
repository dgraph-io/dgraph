package polkadb

import (
	"path/filepath"

	log "github.com/ChainSafe/log15"
)

// Start...
func (dbService *DbService) Start() <-chan error {
	dbService.err = make(<-chan error)
	return dbService.err
}

// Stop kills running BlockDB and StateDB instances
func (dbService *DbService) Stop() <-chan error {
	e := make(chan error)
	// Closing Badger Databases
	err := dbService.StateDB.Db.Close()
	if err != nil {
		e <- err
	}

	err = dbService.BlockDB.Db.Close()
	if err != nil {
		e <- err
	}
	return e
}

// DbService contains both databases for service registry
type DbService struct {
	StateDB *StateDB
	BlockDB *BlockDB

	err <-chan error
}

// NewDatabaseService opens and returns a new DB object
func NewDatabaseService(file string) (*DbService, error) {
	stateDataDir := filepath.Join(file, "state")
	blockDataDir := filepath.Join(file, "block")

	stateDb, err := NewStateDB(stateDataDir)
	if err != nil {
		log.Crit("failed to instantiate StateDB", "error", err)
		return nil, err
	}

	blockDb, err := NewBlockDB(blockDataDir)
	if err != nil {
		log.Crit("failed to instantiate BlockDB", "error", err)
		return nil, err
	}

	return &DbService{
		StateDB: stateDb,
		BlockDB: blockDb,
	}, nil
}
