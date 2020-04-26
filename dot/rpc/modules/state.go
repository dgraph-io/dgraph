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

package modules

import (
	"encoding/hex"
	"net/http"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/runtime"
)

// StateCallRequest holds json fields
type StateCallRequest struct {
	Method string      `json:"method"`
	Data   []byte      `json:"data"`
	Block  common.Hash `json:"block"`
}

// StateChildStorageRequest holds json fields
type StateChildStorageRequest struct {
	ChildStorageKey []byte      `json:"childStorageKey"`
	Key             []byte      `json:"key"`
	Block           common.Hash `json:"block"`
}

// StateStorageKeyRequest holds json fields
type StateStorageKeyRequest struct {
	Key   []byte      `json:"key"`
	Block common.Hash `json:"block"`
}

// StateStorageQueryRequest holds json fields
type StateStorageQueryRequest struct {
	Key   []byte      `json:"key"`
	Block common.Hash `json:"block"`
}

// StateBlockHashQuery is a hash value
type StateBlockHashQuery common.Hash

// StateRuntimeMetadataQuery is a hash value
type StateRuntimeMetadataQuery common.Hash

// StateStorageQueryRangeRequest holds json fields
type StateStorageQueryRangeRequest struct {
	Keys       []byte      `json:"keys"`
	StartBlock common.Hash `json:"startBlock"`
	Block      common.Hash `json:"block"`
}

// StateStorageKeysQuery field to store storage keys
type StateStorageKeysQuery [][]byte

// StateCallResponse holds json fields
type StateCallResponse struct {
	StateCallResponse []byte `json:"stateCallResponse"`
}

// StateKeysResponse field to store the state keys
type StateKeysResponse [][]byte

// StateStorageDataResponse field to store data response
type StateStorageDataResponse []byte

// StateStorageHashResponse is a hash value
type StateStorageHashResponse common.Hash

// StateStorageSizeResponse the default size for response
type StateStorageSizeResponse uint64

// StateStorageKeysResponse field for storage keys
type StateStorageKeysResponse [][]byte

// StateMetadataResponse holds the metadata
//TODO: Determine actual type
type StateMetadataResponse []byte

// StorageChangeSetResponse is the struct that holds the block and changes
type StorageChangeSetResponse struct {
	Block   common.Hash
	Changes []KeyValueOption
}

// KeyValueOption struct holds json fields
type KeyValueOption struct {
	StorageKey  []byte `json:"storageKey"`
	StorageData []byte `json:"storageData"`
}

// StorageKey is the key for the storage
type StorageKey []byte

// StateRuntimeVersionResponse is the runtime version response
type StateRuntimeVersionResponse struct {
	SpecName         string        `json:"specName"`
	ImplName         string        `json:"implName"`
	AuthoringVersion int32         `json:"authoringVersion"`
	SpecVersion      int32         `json:"specVersion"`
	ImplVersion      int32         `json:"implVersion"`
	Apis             []interface{} `json:"apis"`
}

// StateModule is an RPC module providing access to storage API points.
type StateModule struct {
	networkAPI NetworkAPI
	storageAPI StorageAPI
	coreAPI    CoreAPI
}

// NewStateModule creates a new State module.
func NewStateModule(net NetworkAPI, storage StorageAPI, core CoreAPI) *StateModule {
	return &StateModule{
		networkAPI: net,
		storageAPI: storage,
		coreAPI:    core,
	}
}

// Call isn't implemented properly yet.
func (sm *StateModule) Call(r *http.Request, req *StateCallRequest, res *StateCallResponse) {
	_ = sm.networkAPI
	_ = sm.storageAPI
}

// GetChildKeys isn't implemented properly yet.
func (sm *StateModule) GetChildKeys(r *http.Request, req *StateChildStorageRequest, res *StateKeysResponse) {
}

// GetChildStorage isn't implemented properly yet.
func (sm *StateModule) GetChildStorage(r *http.Request, req *StateChildStorageRequest, res *StateStorageDataResponse) {
}

// GetChildStorageHash isn't implemented properly yet.
func (sm *StateModule) GetChildStorageHash(r *http.Request, req *StateChildStorageRequest, res *StateStorageHashResponse) {
}

// GetChildStorageSize isn't implemented properly yet.
func (sm *StateModule) GetChildStorageSize(r *http.Request, req *StateChildStorageRequest, res *StateStorageSizeResponse) {
}

// GetKeys isn't implemented properly yet.
func (sm *StateModule) GetKeys(r *http.Request, req *StateStorageKeyRequest, res *StateStorageKeysResponse) {
}

// GetMetadata isn't implemented properly yet.
func (sm *StateModule) GetMetadata(r *http.Request, req *StateRuntimeMetadataQuery, res *StateMetadataResponse) {
}

// GetRuntimeVersion Get the runtime version at a given block.
//  If no block hash is provided, the latest version gets returned.
// TODO currently only returns latest version, add functionality to lookup runtime by block hash
func (sm *StateModule) GetRuntimeVersion(r *http.Request, req *StateBlockHashQuery, res *StateRuntimeVersionResponse) error {
	rtVersion, err := sm.coreAPI.GetRuntimeVersion()
	res.SpecName = string(rtVersion.RuntimeVersion.Spec_name)
	res.ImplName = string(rtVersion.RuntimeVersion.Impl_name)
	res.AuthoringVersion = rtVersion.RuntimeVersion.Authoring_version
	res.SpecVersion = rtVersion.RuntimeVersion.Spec_version
	res.ImplVersion = rtVersion.RuntimeVersion.Impl_version
	res.Apis = convertAPIs(rtVersion.API)

	return err
}

// GetStorage isn't implemented properly yet.
func (sm *StateModule) GetStorage(r *http.Request, req *StateStorageQueryRequest, res *StateStorageDataResponse) {
}

// GetStorageHash isn't implemented properly yet.
func (sm *StateModule) GetStorageHash(r *http.Request, req *StateStorageQueryRequest, res *StateStorageHashResponse) {
}

// GetStorageSize isn't implemented properly yet.
func (sm *StateModule) GetStorageSize(r *http.Request, req *StateStorageQueryRequest, res *StateStorageSizeResponse) {
}

// QueryStorage isn't implemented properly yet.
func (sm *StateModule) QueryStorage(r *http.Request, req *StateStorageQueryRangeRequest, res *StorageChangeSetResponse) {
}

// SubscribeRuntimeVersion isn't implemented properly yet.
// TODO make this actually a subscription that pushes data
func (sm *StateModule) SubscribeRuntimeVersion(r *http.Request, req *StateStorageQueryRangeRequest, res *StateRuntimeVersionResponse) error {
	return sm.GetRuntimeVersion(r, nil, res)
}

// SubscribeStorage isn't implemented properly yet.
func (sm *StateModule) SubscribeStorage(r *http.Request, req *StateStorageQueryRangeRequest, res *StorageChangeSetResponse) {
}

func convertAPIs(in []*runtime.API_Item) []interface{} {
	ret := make([]interface{}, 0)
	for _, item := range in {
		encStr := hex.EncodeToString(item.Name)
		ret = append(ret, []interface{}{"0x" + encStr, item.Ver})
	}
	return ret
}
