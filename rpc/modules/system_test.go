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
	"testing"

	"github.com/ChainSafe/gossamer/internal/api"
)

var (
	testRuntimeVersion = "1.2.3"
)

type mockruntimeApi struct{}

func (a *mockruntimeApi) Version() string {
	return testRuntimeVersion
}

func newMockApi() *api.Api {
	runtimeApi := &mockruntimeApi{}

	return &api.Api{
		System: api.NewSystemModule(nil, runtimeApi),
	}
}

func TestSystemModule_Version(t *testing.T) {
	sys := NewSystemModule(newMockApi())

	vres := &SystemVersionResponse{}
	err := sys.Version(nil, nil, vres)
	if err != nil {
		t.Fatal(err)
	}
	if vres.Version != testRuntimeVersion {
		t.Fatalf("System.Version: expected: %s got: %s\n", vres.Version, testRuntimeVersion)
	}
}
