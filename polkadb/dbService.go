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

	"github.com/ChainSafe/gossamer/internal/services"
)

var _ services.Service = &DbService{}

// DbService contains both databases for service registry
type DbService struct {
	path    string
	StateDB *StateDB
	BlockDB *BlockDB
}

// NewDbService opens and returns a new DB object
func NewDbService(path string) (*DbService, error) {
	return &DbService{
		path:    path,
		StateDB: nil,
		BlockDB: nil,
	}, nil
}

// Start instantiates the StateDB and BlockDB if they do not exist
func (s *DbService) Start() error {
	if s.StateDB != nil || s.BlockDB != nil {
		return nil
	}

	stateDataDir := filepath.Join(s.path, "state")
	blockDataDir := filepath.Join(s.path, "block")

	stateDb, err := NewStateDB(stateDataDir)
	if err != nil {
		return err
	}

	blockDb, err := NewBlockDB(blockDataDir)
	if err != nil {
		return err
	}

	s.BlockDB = blockDb
	s.StateDB = stateDb

	return nil
}

// Stop kills running BlockDB and StateDB instances
func (s *DbService) Stop() error {
	// Closing Badger Databases
	err := s.StateDB.Db.Close()
	if err != nil {
		return err
	}

	err = s.BlockDB.Db.Close()
	if err != nil {
		return err
	}
	return nil
}
