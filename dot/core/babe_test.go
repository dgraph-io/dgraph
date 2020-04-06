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

package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// test finalizeBabeSession
func TestFinalizeBabeSession(t *testing.T) {
	s := newTestServiceWithFirstBlock(t)

	number, err := s.blockState.BestBlockNumber()
	require.Nil(t, err)

	header, err := s.blockState.BestBlockHeader()
	require.Nil(t, err)

	err = s.handleBlockDigest(header)
	require.Nil(t, err)

	require.Equal(t, number, s.firstBlock)

	err = s.finalizeBabeSession()
	require.Nil(t, err)

	require.Nil(t, s.firstBlock)
}

// test initializeBabeSession
func TestInitializeBabeSession(t *testing.T) {
	s := newTestServiceWithFirstBlock(t)

	number, err := s.blockState.BestBlockNumber()
	require.Nil(t, err)

	header, err := s.blockState.BestBlockHeader()
	require.Nil(t, err)

	err = s.handleBlockDigest(header)
	require.Nil(t, err)

	require.Equal(t, number, s.firstBlock)

	epochNumber := s.epochNumber

	err = s.finalizeBabeSession()
	require.Nil(t, err)

	require.Nil(t, s.firstBlock)

	bs, err := s.initializeBabeSession()
	require.Nil(t, err)

	require.Equal(t, epochNumber+1, s.epochNumber)
	require.Nil(t, s.firstBlock)

	require.NotNil(t, bs)
}
