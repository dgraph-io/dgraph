// Copyright 2020 ChainSafe Systems (ON) Corp.
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
	"fmt"
	"sort"
	"testing"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/stretchr/testify/require"
)

func TestStateModule_GetRuntimeVersion(t *testing.T) {
	// expected results based on responses from prior tests
	expected := StateRuntimeVersionResponse{
		SpecName:         "test",
		ImplName:         "parity-test",
		AuthoringVersion: 1,
		SpecVersion:      1,
		ImplVersion:      1,
		Apis: []interface{}{[]interface{}{"0xdf6acb689907609b", int32(2)},
			[]interface{}{"0x37e397fc7c91f5e4", int32(1)},
			[]interface{}{"0xd2bc9897eed08f15", int32(1)},
			[]interface{}{"0x40fe3ad401f8959a", int32(3)},
			[]interface{}{"0xc6e9a76309f39b09", int32(1)},
			[]interface{}{"0xdd718d5cc53262d4", int32(1)},
			[]interface{}{"0xcbca25e39f142387", int32(1)},
			[]interface{}{"0xf78b278be53f454c", int32(1)},
			[]interface{}{"0xab3c0572291feb8b", int32(1)}},
	}
	sm := setupStateModule(t)
	res := StateRuntimeVersionResponse{}
	err := sm.GetRuntimeVersion(nil, nil, &res)
	require.Nil(t, err)

	require.Equal(t, expected, res)
}

func TestStateModule_GetPairs_All(t *testing.T) {
	sm := setupStateModule(t)
	expected := []interface{}{[]string{"0x3a6b657931", "0x76616c756531"}, []string{"0x3a6b657932", "0x76616c756532"}}
	req := []string{""}
	var res []interface{}

	err := sm.GetPairs(nil, &req, &res)
	require.NoError(t, err)
	sort.Slice(res, func(i, j int) bool {
		return res[i].([]string)[0] < res[j].([]string)[0]
	})
	require.Equal(t, expected, res)

}

func TestStateModule_GetPairs_One(t *testing.T) {
	sm := setupStateModule(t)
	expected := []interface{}{[]string{"0x3a6b657931", "0x76616c756531"}}
	req := []string{"0x3a6b657931"}
	var res []interface{}

	err := sm.GetPairs(nil, &req, &res)
	require.NoError(t, err)
	require.Equal(t, expected, res)

}

func TestStateModule_GetPairs_None(t *testing.T) {
	sm := setupStateModule(t)
	expected := []interface{}{}
	req := []string{"0x00"}
	var res []interface{}

	err := sm.GetPairs(nil, &req, &res)
	require.NoError(t, err)
	require.Equal(t, expected, res)

}

func TestStateModule_GetStorage(t *testing.T) {
	sm := setupStateModule(t)
	expected := []byte(`value1`)
	req := []string{"0x3a6b657931"} // :key1
	var res interface{}

	err := sm.GetStorage(nil, &req, &res)
	require.NoError(t, err)

	hex, err := common.HexToBytes(fmt.Sprintf("%v", res))

	require.NoError(t, err)
	require.Equal(t, expected, hex)
}

func TestStateModule_GetStorage_NotFound(t *testing.T) {
	sm := setupStateModule(t)

	req := []string{"0x666f6f"} // foo
	var res interface{}

	err := sm.GetStorage(nil, &req, &res)
	require.NoError(t, err)
	require.Nil(t, res)
}

func TestStateModule_GetStorageHash(t *testing.T) {
	sm := setupStateModule(t)
	expected := common.BytesToHash([]byte(`value1`)).String()
	req := []string{"0x3a6b657931"} // :key1
	var res interface{}

	err := sm.GetStorageHash(nil, &req, &res)
	require.NoError(t, err)
	require.Equal(t, expected, res)
}

func TestStateModule_GetStorageHash_NotFound(t *testing.T) {
	sm := setupStateModule(t)
	req := []string{"0x666f6f"} // foo
	var res interface{}

	err := sm.GetStorageHash(nil, &req, &res)
	require.NoError(t, err)
	require.Equal(t, nil, res)
}

func TestStateModule_GetStorageSize(t *testing.T) {
	sm := setupStateModule(t)
	expected := 6
	req := []string{"0x3a6b657931"} // :key1
	var res interface{}

	err := sm.GetStorageSize(nil, &req, &res)
	require.NoError(t, err)
	require.Equal(t, expected, res)
}

func TestStateModule_GetStorageSize_NotFound(t *testing.T) {
	sm := setupStateModule(t)
	req := []string{"0x666f6f"} // foo
	var res interface{}

	err := sm.GetStorageSize(nil, &req, &res)
	require.NoError(t, err)
	require.Equal(t, nil, res)
}

func TestStateModule_GetMetadata(t *testing.T) {
	sm := setupStateModule(t)
	var res string
	err := sm.GetMetadata(nil, nil, &res)

	// currently this is generating an error because runtime has not implemented Metadata_metadata yet
	//  expect this to change when runtime changes
	require.Equal(t, "0x", res)
	require.EqualError(t, err, "Failed to call the `Metadata_metadata` exported function.")

}

func setupStateModule(t *testing.T) *StateModule {
	// setup service
	net := newNetworkService(t)
	chain := newTestChainService(t)
	// init storage with test data
	err := chain.Storage.SetStorage([]byte(`:key1`), []byte(`value1`))
	require.NoError(t, err)
	err = chain.Storage.SetStorage([]byte(`:key2`), []byte(`value2`))
	require.NoError(t, err)

	core := newCoreService(t)
	return NewStateModule(net, chain.Storage, core)
}
