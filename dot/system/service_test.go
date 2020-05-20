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

package system

import (
	"testing"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/stretchr/testify/require"
)

func TestService_NodeName(t *testing.T) {
	svc := newTestService()

	name := svc.NodeName()
	require.Equal(t, "gssmr", name)
}

func TestService_SystemName(t *testing.T) {
	svc := newTestService()

	name := svc.SystemName()
	require.Equal(t, "gossamer", name)
}

func TestService_SystemVersion(t *testing.T) {
	svc := newTestService()
	ver := svc.SystemVersion()
	require.Equal(t, "0.0.1", ver)
}

func TestService_Properties(t *testing.T) {
	expected := make(map[string]interface{})
	expected["ss58Format"] = 2
	expected["tokenDecimals"] = 12
	expected["tokenSymbol"] = "KSM"

	svc := newTestService()
	props := svc.Properties()
	require.Equal(t, expected, props)
}

func TestService_Start(t *testing.T) {
	svc := newTestService()
	err := svc.Start()
	require.NoError(t, err)
}

func TestService_Stop(t *testing.T) {
	svc := newTestService()
	err := svc.Stop()
	require.NoError(t, err)
}

func newTestService() *Service {
	sysProps := make(map[string]interface{})
	sysProps["ss58Format"] = 2
	sysProps["tokenDecimals"] = 12
	sysProps["tokenSymbol"] = "KSM"

	sysInfo := &types.SystemInfo{
		SystemName:       "gossamer",
		SystemVersion:    "0.0.1",
		NodeName:         "gssmr",
		SystemProperties: sysProps,
	}
	return NewService(sysInfo)
}
