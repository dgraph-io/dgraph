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

package wasmer

import (
	"testing"

	"github.com/ChainSafe/gossamer/lib/runtime"

	"github.com/stretchr/testify/require"
)

func TestInstance_Version_NodeRuntime(t *testing.T) {
	// https://github.com/paritytech/substrate/blob/7b1d822446982013fa5b7ad5caff35ca84f8b7d0/core/test-runtime/src/lib.rs#L73
	expected := &runtime.Version{
		Spec_name:         []byte("node"),
		Impl_name:         []byte("substrate-node"),
		Authoring_version: 10,
		Spec_version:      193,
		Impl_version:      193,
	}

	instance := NewTestInstance(t, runtime.NODE_RUNTIME)

	ret, err := instance.exec(runtime.CoreVersion, []byte{})
	require.Nil(t, err)

	version := &runtime.VersionAPI{
		RuntimeVersion: &runtime.Version{},
		API:            nil,
	}
	version.Decode(ret)
	require.Nil(t, err)

	t.Logf("Spec_name: %s\n", version.RuntimeVersion.Spec_name)
	t.Logf("Impl_name: %s\n", version.RuntimeVersion.Impl_name)
	t.Logf("Authoring_version: %d\n", version.RuntimeVersion.Authoring_version)
	t.Logf("Spec_version: %d\n", version.RuntimeVersion.Spec_version)
	t.Logf("Impl_version: %d\n", version.RuntimeVersion.Impl_version)

	require.Equal(t, expected, version.RuntimeVersion)
}

func TestInstance_Version_TestRuntime(t *testing.T) {
	// https://github.com/paritytech/substrate/blob/7b1d822446982013fa5b7ad5caff35ca84f8b7d0/core/test-runtime/src/lib.rs#L73
	expected := &runtime.Version{
		Spec_name:         []byte("test"),
		Impl_name:         []byte("parity-test"),
		Authoring_version: 1,
		Spec_version:      1,
		Impl_version:      1,
	}

	instance := NewTestInstance(t, runtime.SUBSTRATE_TEST_RUNTIME)

	ret, err := instance.exec(runtime.CoreVersion, []byte{})
	require.Nil(t, err)

	version := &runtime.VersionAPI{
		RuntimeVersion: &runtime.Version{},
		API:            nil,
	}
	version.Decode(ret)
	require.Nil(t, err)

	t.Logf("Spec_name: %s\n", version.RuntimeVersion.Spec_name)
	t.Logf("Impl_name: %s\n", version.RuntimeVersion.Impl_name)
	t.Logf("Authoring_version: %d\n", version.RuntimeVersion.Authoring_version)
	t.Logf("Spec_version: %d\n", version.RuntimeVersion.Spec_version)
	t.Logf("Impl_version: %d\n", version.RuntimeVersion.Impl_version)

	require.Equal(t, expected, version.RuntimeVersion)
}

// test used for ensuring runtime exec calls can me made concurrently
func TestConcurrentRuntimeCalls(t *testing.T) {
	instance := NewTestInstance(t, runtime.TEST_RUNTIME)

	// execute 2 concurrent calls to the runtime
	go func() {
		_, _ = instance.exec(runtime.CoreVersion, []byte{})
	}()
	go func() {
		_, _ = instance.exec(runtime.CoreVersion, []byte{})
	}()
}
