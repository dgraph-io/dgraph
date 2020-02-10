package state

import (
	"encoding/json"

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/db"
)

var healthKey = []byte("health")
var networkStateKey = []byte("networkstate")
var peersKey = []byte("peers")

// NetworkDB stores network information in an underlying database
type NetworkDB struct {
	Db db.Database
}

// NetworkState defines fields for manipulating the state of network
type NetworkState struct {
	db *NetworkDB
}

// NewNetworkDB instantiates a badgerDB instance for storing relevant BlockData
func NewNetworkDB(dataDir string) (*NetworkDB, error) {
	db, err := db.NewBadgerDB(dataDir)
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
