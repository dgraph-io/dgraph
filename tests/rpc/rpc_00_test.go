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

package rpc

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"testing"

	"github.com/ChainSafe/gossamer/tests/utils"
	"github.com/stretchr/testify/require"
)

var (
	currentPort = strconv.Itoa(utils.BaseRPCPort)
	rpcSuite    = "rpc"
)

func TestMain(m *testing.M) {
	_, _ = fmt.Fprintln(os.Stdout, "Going to start RPC suite test")

	// Start all tests
	code := m.Run()
	os.Exit(code)
}

type testCase struct {
	description string
	method      string
	params      string
	expected    interface{}
	skip        bool
}

func getResponse(t *testing.T, test *testCase) interface{} {
	if test.skip {
		t.Skip("RPC endpoint not yet implemented")
		return nil
	}

	respBody, err := utils.PostRPC(test.method, utils.NewEndpoint(currentPort), test.params)
	require.Nil(t, err)

	target := reflect.New(reflect.TypeOf(test.expected)).Interface()
	err = utils.DecodeRPC(t, respBody, target)
	require.Nil(t, err, "Could not DecodeRPC", string(respBody))

	require.NotNil(t, target)

	return target
}
