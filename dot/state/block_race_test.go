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
	"runtime"
	"sync"
	"testing"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/trie"

	"github.com/ChainSafe/chaindb"
	"github.com/stretchr/testify/require"
)

func TestConcurrencySetHeader(t *testing.T) {
	threads := runtime.NumCPU()
	dbs := make([]chaindb.Database, threads)
	for i := 0; i < threads; i++ {
		dbs[i] = chaindb.NewMemDatabase()
	}

	pend := new(sync.WaitGroup)
	pend.Add(threads)
	for i := 0; i < threads; i++ {
		go func(index int) {
			defer pend.Done()

			bs := &BlockState{
				db: dbs[index],
			}

			header := &types.Header{
				Number:    big.NewInt(0),
				StateRoot: trie.EmptyHash,
				Digest:    [][]byte{},
			}

			err := bs.SetHeader(header)
			require.Nil(t, err)

			res, err := bs.GetHeader(header.Hash())
			require.Nil(t, err)

			require.Equal(t, header, res)

		}(i)
	}
	pend.Wait()
}
