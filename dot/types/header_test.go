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
	"bytes"
	"math/big"
	"testing"

	"github.com/ChainSafe/gossamer/lib/common"

	"github.com/stretchr/testify/require"
)

func TestDecodeHeader(t *testing.T) {
	header, err := NewHeader(common.Hash{}, big.NewInt(0), common.Hash{}, common.Hash{}, [][]byte{{}})
	require.NoError(t, err)

	enc, err := header.Encode()
	require.NoError(t, err)

	rw := &bytes.Buffer{}
	rw.Write(enc)
	dec, err := new(Header).Decode(rw)
	require.NoError(t, err)
	dec.Hash()
	require.Equal(t, header, dec)
}
