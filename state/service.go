package state

import (
	"path/filepath"
)

type Service struct {
	dbPath  string
	Storage *storageState
	Block   *blockState
	Network *networkState
}

func NewService(path string) *Service {
	return &Service{
		dbPath:  path,
		Storage: nil,
		Block:   nil,
		Network: nil,
	}
}

func (s *Service) Start() error {
	if s.Storage != nil || s.Block != nil {
		return nil
	}

	stateDataDir := filepath.Join(s.dbPath, "state")
	blockDataDir := filepath.Join(s.dbPath, "block")

	storageDb, err := NewStorageState(stateDataDir)
	if err != nil {
		return err
	}

	blockDb, err := NewBlockState(blockDataDir)
	if err != nil {
		return err
	}

	s.Storage = storageDb
	s.Block = blockDb
	s.Network = NewNetworkState()

	return nil
}

func (s *Service) Stop() error {
	// Closing Badger Databases
	err := s.Storage.Db.Db.Close()
	if err != nil {
		return err
	}

	err = s.Block.db.Db.Close()
	if err != nil {
		return err
	}

	return nil
}
