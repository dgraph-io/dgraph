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
	"sync"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/dot/types"

	"github.com/stretchr/testify/require"
)

var testMessageTimeout = time.Second * 3

func TestImportChannel(t *testing.T) {
	bs := newTestBlockState(t, testGenesisHeader)

	ch := make(chan *types.Block, 3)
	id, err := bs.RegisterImportedChannel(ch)
	require.NoError(t, err)

	defer bs.UnregisterImportedChannel(id)

	AddBlocksToState(t, bs, 3)

	for i := 0; i < 3; i++ {
		select {
		case <-ch:
		case <-time.After(testMessageTimeout):
			t.Fatal("did not receive imported block")
		}
	}
}

func TestFinalizedChannel(t *testing.T) {
	bs := newTestBlockState(t, testGenesisHeader)

	ch := make(chan *types.Header, 3)
	id, err := bs.RegisterFinalizedChannel(ch)
	require.NoError(t, err)

	defer bs.UnregisterFinalizedChannel(id)

	chain, _ := AddBlocksToState(t, bs, 3)

	for _, b := range chain {
		bs.SetFinalizedHash(b.Hash(), 0, 0)
	}

	for i := 0; i < 1; i++ {
		select {
		case <-ch:
		case <-time.After(testMessageTimeout):
			t.Fatal("did not receive finalized block")
		}
	}
}

func TestImportChannel_Multi(t *testing.T) {
	bs := newTestBlockState(t, testGenesisHeader)

	num := 5
	chs := make([]chan *types.Block, num)
	ids := make([]byte, num)

	var err error
	for i := 0; i < num; i++ {
		chs[i] = make(chan *types.Block)
		ids[i], err = bs.RegisterImportedChannel(chs[i])
		require.NoError(t, err)
	}

	var wg sync.WaitGroup
	wg.Add(num)

	for i, ch := range chs {

		go func(i int, ch chan *types.Block) {
			select {
			case b := <-ch:
				require.Equal(t, big.NewInt(1), b.Header.Number)
			case <-time.After(testMessageTimeout):
				t.Error("did not receive imported block: ch=", i)
			}
			wg.Done()
		}(i, ch)

	}

	time.Sleep(time.Millisecond * 10)
	AddBlocksToState(t, bs, 1)
	wg.Wait()

	for _, id := range ids {
		bs.UnregisterImportedChannel(id)
	}
}

func TestFinalizedChannel_Multi(t *testing.T) {
	bs := newTestBlockState(t, testGenesisHeader)

	num := 5
	chs := make([]chan *types.Header, num)
	ids := make([]byte, num)

	var err error
	for i := 0; i < num; i++ {
		chs[i] = make(chan *types.Header)
		ids[i], err = bs.RegisterFinalizedChannel(chs[i])
		require.NoError(t, err)
	}

	chain, _ := AddBlocksToState(t, bs, 1)

	var wg sync.WaitGroup
	wg.Add(num)

	for i, ch := range chs {

		go func(i int, ch chan *types.Header) {
			select {
			case <-ch:
			case <-time.After(testMessageTimeout):
				t.Error("did not receive finalized block: ch=", i)
			}
			wg.Done()
		}(i, ch)

	}

	time.Sleep(time.Millisecond * 10)
	bs.SetFinalizedHash(chain[0].Hash(), 0, 0)
	wg.Wait()

	for _, id := range ids {
		bs.UnregisterFinalizedChannel(id)
	}
}
