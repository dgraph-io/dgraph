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

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/database"
)

var healthKey = []byte("health")
var networkStateKey = []byte("networkstate")
var peersKey = []byte("peers")

// NetworkDB stores network information in an underlying database
type NetworkDB struct {
	Db database.Database
}

// NetworkState defines fields for manipulating the state of network
type NetworkState struct {
	db *NetworkDB
}

// NewNetworkDB instantiates a badgerDB instance for storing relevant BlockData
func NewNetworkDB(dataDir string) (*NetworkDB, error) {
	db, err := database.NewBadgerDB(dataDir)
	if err != nil {
		return nil, err
	}
	return &NetworkDB{
		db,
	}, nil
}

// NewNetworkState creates NetworkState with a network database in DataDir
func NewNetworkState(dataDir string) (*NetworkState, error) {
	networkDb, err := NewNetworkDB(dataDir)
	if err != nil {
		return nil, err
	}
	return &NetworkState{
		db: networkDb,
	}, nil
}

// GetHealth retrieves network health from the database
func (ns *NetworkState) GetHealth() (*common.Health, error) {
	res := new(common.Health)
	data, err := ns.db.Db.Get(healthKey)
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
	err = ns.db.Db.Put(healthKey, enc)
	return err
}

// GetNetworkState retrieves network state from the database
func (ns *NetworkState) GetNetworkState() (*common.NetworkState, error) {
	res := new(common.NetworkState)
	data, err := ns.db.Db.Get(networkStateKey)
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
	err = ns.db.Db.Put(networkStateKey, enc)
	return err
}

// GetPeers retrieves network state from the database
func (ns *NetworkState) GetPeers() (*[]common.PeerInfo, error) {
	res := new([]common.PeerInfo)
	data, err := ns.db.Db.Get(peersKey)
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
	err = ns.db.Db.Put(peersKey, enc)
	return err
}
