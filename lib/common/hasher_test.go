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

package common_test

import (
	"testing"

	"github.com/ChainSafe/gossamer/lib/common"

	"github.com/stretchr/testify/require"
)

func TestBlake2b218_EmptyHash(t *testing.T) {
	// test case from https://github.com/noot/blake2b_test which uses the blake2-rfp rust crate
	// also see https://github.com/paritytech/substrate/blob/master/core/primitives/src/hashing.rs
	in := []byte{}
	h, err := common.Blake2b128(in)
	require.NoError(t, err)

	expected, err := common.HexToBytes("0xcae66941d9efbd404e4d88758ea67670")
	require.NoError(t, err)
	require.Equal(t, expected, h)
}

func TestBlake128(t *testing.T) {
	in := []byte("static")
	h, err := common.Blake2b128(in)
	require.NoError(t, err)

	expected, err := common.HexToBytes("0x440973e4e50902f1d0ec97de357eb2fd")
	require.NoError(t, err)
	require.Equal(t, expected, h)
}

func TestBlake2bHash_EmptyHash(t *testing.T) {
	// test case from https://github.com/noot/blake2b_test which uses the blake2-rfp rust crate
	// also see https://github.com/paritytech/substrate/blob/master/core/primitives/src/hashing.rs
	in := []byte{}
	h, err := common.Blake2bHash(in)
	require.NoError(t, err)

	expected, err := common.HexToHash("0x0e5751c026e543b2e8ab2eb06099daa1d1e5df47778f7787faab45cdf12fe3a8")
	require.NoError(t, err)
	require.Equal(t, expected, h)
}

func TestKeccak256_EmptyHash(t *testing.T) {
	// test case from https://github.com/debris/tiny-keccak/blob/master/tests/keccak.rs#L4
	in := []byte{}
	h, err := common.Keccak256(in)
	require.NoError(t, err)

	expected, err := common.HexToHash("0xc5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470")
	require.NoError(t, err)
	require.Equal(t, expected, h)
}

func TestKeccak256(t *testing.T) {
	in := []byte("static")
	h, err := common.Keccak256(in)
	require.NoError(t, err)

	expected, err := common.HexToHash("0xd517392f8119f79c1623774b9346e00104a1d193f1fa641e6e659bf323c37967")
	require.NoError(t, err)
	require.Equal(t, expected, h)
}
