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

package types

import (
	"testing"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/stretchr/testify/require"
)

func TestBodyToExtrinsics(t *testing.T) {
	exts := []Extrinsic{{1, 2, 3}, {7, 8, 9, 0}, {0xa, 0xb}}

	body, err := NewBodyFromExtrinsics(exts)
	require.NoError(t, err)

	res, err := body.AsExtrinsics()
	require.NoError(t, err)
	require.Equal(t, exts, res)
}

func TestNewBodyFromExtrinsicStrings(t *testing.T) {
	strs := []string{"0xabcd", "0xff9988", "0x7654acdf"}
	body, err := NewBodyFromExtrinsicStrings(strs)
	require.NoError(t, err)

	exts, err := body.AsExtrinsics()
	require.NoError(t, err)

	for i, e := range exts {
		b, err := common.HexToBytes(strs[i])
		require.NoError(t, err)
		require.Equal(t, []byte(e), b)
	}
}

func TestNewBodyFromExtrinsicStrings_Mixed(t *testing.T) {
	strs := []string{"0xabcd", "0xff9988", "noot"}
	body, err := NewBodyFromExtrinsicStrings(strs)
	require.NoError(t, err)

	exts, err := body.AsExtrinsics()
	require.NoError(t, err)

	for i, e := range exts {
		b, err := common.HexToBytes(strs[i])
		if err == common.ErrNoPrefix {
			b = []byte(strs[i])
		} else if err != nil {
			t.Fatal(err)
		}
		require.Equal(t, []byte(e), b)
	}
}
