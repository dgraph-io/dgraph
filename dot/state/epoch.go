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

package state

import (
	"encoding/binary"
	"errors"

	"github.com/ChainSafe/chaindb"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/scale"
)

var (
	epochPrefix     = []byte("epoch")
	currentEpochKey = []byte("current")
	epochInfoPrefix = []byte("epochinfo")
)

func epochInfoKey(epoch uint64) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, epoch)
	return append(epochInfoPrefix, buf...)
}

// EpochDB stores epoch info in an underlying Database
type EpochDB struct {
	db chaindb.Database
}

// Put appends `epoch` to the key and sets the key-value pair in the db
func (db *EpochDB) Put(key, value []byte) error {
	key = append(epochPrefix, key...)
	return db.db.Put(key, value)
}

// Get appends `epoch` to the key and retrieves the value from the db
func (db *EpochDB) Get(key []byte) ([]byte, error) {
	key = append(epochPrefix, key...)
	return db.db.Get(key)
}

// Delete deletes a key from the db
func (db *EpochDB) Delete(key []byte) error {
	key = append(epochPrefix, key...)
	return db.db.Del(key)
}

// Has appends `epoch` to the key and checks for existence in the db
func (db *EpochDB) Has(key []byte) (bool, error) {
	key = append(epochPrefix, key...)
	return db.db.Has(key)
}

// newEpochDB instantiates a badgerDB instance for stssoring relevant epoch info
func newEpochDB(db chaindb.Database) *EpochDB {
	return &EpochDB{
		db,
	}
}

// EpochState tracks information related to each epoch
type EpochState struct {
	db *EpochDB
}

// NewEpochStateFromGenesis returns a new EpochState given information for the first epoch, fetched from the runtime
func NewEpochStateFromGenesis(db chaindb.Database, info *types.EpochInfo) (*EpochState, error) {
	epochDB := newEpochDB(db)
	err := epochDB.Put(currentEpochKey, []byte{1, 0, 0, 0, 0, 0, 0, 0})
	if err != nil {
		return nil, err
	}

	s := &EpochState{
		db: epochDB,
	}

	err = s.SetEpochInfo(1, info)
	if err != nil {
		return nil, err
	}

	return s, nil
}

// NewEpochState returns a new EpochState
func NewEpochState(db chaindb.Database) *EpochState {
	epochDB := newEpochDB(db)
	return &EpochState{
		db: epochDB,
	}
}

// SetCurrentEpoch sets the current epoch
func (s *EpochState) SetCurrentEpoch(epoch uint64) error {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, epoch)
	return s.db.Put(currentEpochKey, buf)
}

// GetCurrentEpoch returns the current epoch
func (s *EpochState) GetCurrentEpoch() (uint64, error) {
	b, err := s.db.Get(currentEpochKey)
	if err != nil {
		return 0, err
	}

	return binary.LittleEndian.Uint64(b), nil
}

// SetEpochInfo sets the epoch info for a given epoch
func (s *EpochState) SetEpochInfo(epoch uint64, info *types.EpochInfo) error {
	enc, err := scale.Encode(info)
	if err != nil {
		return err
	}

	return s.db.Put(epochInfoKey(epoch), enc)
}

// GetEpochInfo returns the epoch info for a given epoch
func (s *EpochState) GetEpochInfo(epoch uint64) (*types.EpochInfo, error) {
	enc, err := s.db.Get(epochInfoKey(epoch))
	if err != nil {
		return nil, err
	}

	info, err := scale.Decode(enc, new(types.EpochInfo))
	if err != nil {
		return nil, err
	}

	return info.(*types.EpochInfo), nil
}

// HasEpochInfo returns whether epoch info exists for a given epoch
func (s *EpochState) HasEpochInfo(epoch uint64) (bool, error) {
	return s.db.Has(epochInfoKey(epoch))
}

// GetStartSlotForEpoch returns the first slot in the given epoch.
// If 0 is passed as the epoch, it returns the start slot for the current epoch.
func (s *EpochState) GetStartSlotForEpoch(epoch uint64) (uint64, error) {
	curr, err := s.GetCurrentEpoch()
	if err != nil {
		return 0, nil
	}

	if epoch == 0 {
		// epoch 0 doesn't exist, use 0 for latest epoch
		epoch = curr
	}

	if epoch == 1 {
		return 1, nil
	}

	if epoch > curr {
		return 0, errors.New("epoch in future")
	}

	slot := uint64(0)

	// start at epoch 1
	for i := uint64(1); i < epoch; i++ {
		info, err := s.GetEpochInfo(i)
		if err != nil {
			return 0, err
		}

		slot += info.Duration
	}

	return slot + 1, nil
}
