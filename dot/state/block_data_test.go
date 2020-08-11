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

package state

import (
	"math/big"
	"testing"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/common/optional"
	"github.com/ChainSafe/gossamer/lib/trie"

	"github.com/stretchr/testify/require"
)

func TestGetSet_ReceiptMessageQueue_Justification(t *testing.T) {
	s := newTestBlockState(t, nil)
	require.NotNil(t, s)

	var genesisHeader = &types.Header{
		Number:    big.NewInt(0),
		StateRoot: trie.EmptyHash,
	}

	hash := common.NewHash([]byte{0})
	body := optional.CoreBody{0xa, 0xb, 0xc, 0xd}

	parentHash := genesisHeader.Hash()

	stateRoot, err := common.HexToHash("0x2747ab7c0dc38b7f2afba82bd5e2d6acef8c31e09800f660b75ec84a7005099f")
	require.Nil(t, err)

	extrinsicsRoot, err := common.HexToHash("0x03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314")
	require.Nil(t, err)

	header := &types.Header{
		ParentHash:     parentHash,
		Number:         big.NewInt(1),
		StateRoot:      stateRoot,
		ExtrinsicsRoot: extrinsicsRoot,
		Digest:         [][]byte{},
	}

	bds := []*types.BlockData{{
		Hash:          header.Hash(),
		Header:        header.AsOptional(),
		Body:          types.NewBody([]byte{}).AsOptional(),
		Receipt:       optional.NewBytes(false, nil),
		MessageQueue:  optional.NewBytes(false, nil),
		Justification: optional.NewBytes(false, nil),
	}, {
		Hash:          hash,
		Header:        optional.NewHeader(false, nil),
		Body:          optional.NewBody(true, body),
		Receipt:       optional.NewBytes(true, []byte("asdf")),
		MessageQueue:  optional.NewBytes(true, []byte("ghjkl")),
		Justification: optional.NewBytes(true, []byte("qwerty")),
	}}

	for _, blockdata := range bds {

		err := s.CompareAndSetBlockData(blockdata)
		require.Nil(t, err)

		// test Receipt
		if blockdata.Receipt.Exists() {
			receipt, err := s.GetReceipt(blockdata.Hash)
			require.Nil(t, err)
			require.Equal(t, blockdata.Receipt.Value(), receipt)
		}

		// test MessageQueue
		if blockdata.MessageQueue.Exists() {
			messageQueue, err := s.GetMessageQueue(blockdata.Hash)
			require.Nil(t, err)
			require.Equal(t, blockdata.MessageQueue.Value(), messageQueue)
		}

		// test Justification
		if blockdata.Justification.Exists() {
			justification, err := s.GetJustification(blockdata.Hash)
			require.Nil(t, err)
			require.Equal(t, blockdata.Justification.Value(), justification)
		}
	}
}
