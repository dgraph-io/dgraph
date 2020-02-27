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
	"encoding/json"
	"fmt"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/database"
)

var networkPrefix = []byte("network")

var healthKey = []byte("health")
var networkStateKey = []byte("networkstate")
var peersKey = []byte("peers")

// NetworkDB stores network information in an underlying database
type NetworkDB struct {
	db database.Database
}

// Put appends `network` to the key and sets the key-value pair in the db
func (networkDB *NetworkDB) Put(key, value []byte) error {
	key = append(networkPrefix, key...)
	return networkDB.db.Put(key, value)
}

// Get appends `network` to the key and retrieves the value from the db
func (networkDB *NetworkDB) Get(key []byte) ([]byte, error) {
	key = append(networkPrefix, key...)
	return networkDB.db.Get(key)
}

// NetworkState defines fields for manipulating the state of network
type NetworkState struct {
	db *NetworkDB
}

// NewNetworkDB instantiates a badgerDB instance for storing relevant BlockData
func NewNetworkDB(db database.Database) *NetworkDB {
	return &NetworkDB{
		db,
	}
}

// NewNetworkState creates NetworkState with a network database in DataDir
func NewNetworkState(db database.Database) (*NetworkState, error) {
	if db == nil {
		return nil, fmt.Errorf("cannot have nil database")
	}

	return &NetworkState{
		db: NewNetworkDB(db),
	}, nil
}

// GetHealth retrieves network health from the database
func (ns *NetworkState) GetHealth() (*common.Health, error) {
	res := new(common.Health)
	data, err := ns.db.Get(healthKey)
	if err != nil {
		return res, err
	}
	err = json.Unmarshal(data, res)
	return res, err
}

// SetHealth sets network health in the database
func (ns *NetworkState) SetHealth(health *common.Health) error {
	enc, err := json.Marshal(health)
	if err != nil {
		return err
	}
	err = ns.db.Put(healthKey, enc)
	return err
}

// GetNetworkState retrieves network state from the database
func (ns *NetworkState) GetNetworkState() (*common.NetworkState, error) {
	res := new(common.NetworkState)
	data, err := ns.db.Get(networkStateKey)
	if err != nil {
		return res, err
	}
	err = json.Unmarshal(data, res)
	return res, err
}

// SetNetworkState sets network state in the database
func (ns *NetworkState) SetNetworkState(networkState *common.NetworkState) error {
	enc, err := json.Marshal(networkState)
	if err != nil {
		return err
	}
	err = ns.db.Put(networkStateKey, enc)
	return err
}

// GetPeers retrieves network state from the database
func (ns *NetworkState) GetPeers() (*[]common.PeerInfo, error) {
	res := new([]common.PeerInfo)
	data, err := ns.db.Get(peersKey)
	if err != nil {
		return res, err
	}
	err = json.Unmarshal(data, res)
	return res, err
}

// SetPeers sets network state in the database
func (ns *NetworkState) SetPeers(peers *[]common.PeerInfo) error {
	enc, err := json.Marshal(peers)
	if err != nil {
		return err
	}
	err = ns.db.Put(peersKey, enc)
	return err
}
