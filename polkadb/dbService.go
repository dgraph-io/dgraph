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

package polkadb

import (
	"path/filepath"
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
		return nil, err
	}

	blockDb, err := NewBlockDB(blockDataDir)
	if err != nil {
		return nil, err
	}

	return &DbService{
		StateDB: stateDb,
		BlockDB: blockDb,
	}, nil
}
