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

package variadic

import (
	"encoding/binary"
	"testing"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/stretchr/testify/require"
)

func TestNewUint64OrHash(t *testing.T) {
	hash, err := common.HexToHash("0xdcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b")
	if err != nil {
		t.Fatal(err)
	}

	res, err := NewUint64OrHash(hash)
	if err != nil {
		t.Fatal(err)
	}

	if resValue, ok := res.Value().(common.Hash); !ok || resValue != hash {
		t.Fatalf("Fail: got %x expected %x", resValue, hash)
	}

	num := 77

	res, err = NewUint64OrHash(num)
	if err != nil {
		t.Fatal(err)
	}

	if resValue, ok := res.Value().(uint64); !ok || resValue != uint64(num) {
		t.Fatalf("Fail: got %d expected %d", resValue, num)
	}

	res, err = NewUint64OrHash(uint64(num))
	if err != nil {
		t.Fatal(err)
	}

	if resValue, ok := res.Value().(uint64); !ok || resValue != uint64(num) {
		t.Fatalf("Fail: got %d expected %d", resValue, uint64(num))
	}
}

func TestNewUint64OrHashFromBytes(t *testing.T) {
	genesisHash, err := common.HexToBytes("0xdcd1346701ca8396496e52aa2785b1748deb6db09551b72159dcb3e08991025b")
	if err != nil || genesisHash == nil {
		t.Fatal(err)
	}

	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(1))

	for _, x := range []struct {
		description     string
		targetHash      []byte
		targetFirstByte uint8
		expectedType    interface{}
	}{
		{
			description:     "block request with genesis hash type 0",
			targetHash:      genesisHash,
			targetFirstByte: 0,
			expectedType:    common.Hash{},
		},
		{
			description:     "block request with Block Number int type 1",
			targetHash:      buf,
			targetFirstByte: 1,
			expectedType:    (uint64)(0),
		},
	} {
		t.Run(x.description, func(t *testing.T) {
			data := append([]byte{x.targetFirstByte}, x.targetHash...)

			uint64OrHash := NewUint64OrHashFromBytes(data)
			require.Nil(t, err)
			require.NotNil(t, uint64OrHash)
			require.IsType(t, x.expectedType, uint64OrHash.Value())
			if x.expectedType == (uint64)(0) {
				startingBlockByteArray := make([]byte, 8)
				binary.LittleEndian.PutUint64(startingBlockByteArray, uint64OrHash.Value().(uint64))
				require.Equal(t, x.targetHash, startingBlockByteArray)
			} else {
				require.Equal(t, common.NewHash(x.targetHash), uint64OrHash.Value())
			}
		})
	}

}
