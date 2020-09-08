// Copyright 2020 ChainSafe Systems (ON) Corp.
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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStorageState_RegisterStorageChangeChannel(t *testing.T) {
	ss := newTestStorageState(t)

	ch := make(chan *KeyValue, 3)
	id, err := ss.RegisterStorageChangeChannel(ch)
	require.NoError(t, err)

	defer ss.UnregisterStorageChangeChannel(id)

	// three storage change events
	ss.setStorage(nil, []byte("mackcom"), []byte("wuz here"))
	ss.setStorage(nil, []byte("key1"), []byte("value1"))
	ss.setStorage(nil, []byte("key1"), []byte("value2"))

	for i := 0; i < 3; i++ {
		select {
		case <-ch:
		case <-time.After(testMessageTimeout):
			t.Fatal("did not receive storage change message")
		}
	}
}

func TestStorageState_RegisterStorageChangeChannel_Multi(t *testing.T) {
	ss := newTestStorageState(t)

	num := 5
	chs := make([]chan *KeyValue, num)
	ids := make([]byte, num)

	var err error
	for i := 0; i < num; i++ {
		chs[i] = make(chan *KeyValue)
		ids[i], err = ss.RegisterStorageChangeChannel(chs[i])
		require.NoError(t, err)
	}

	key1 := []byte("key1")
	ss.setStorage(nil, key1, []byte("value1"))

	var wg sync.WaitGroup
	wg.Add(num)

	for i, ch := range chs {

		go func(i int, ch chan *KeyValue) {
			select {
			case c := <-ch:
				require.Equal(t, key1, c.Key)
				wg.Done()
			case <-time.After(testMessageTimeout):
				t.Error("did not receive storage change: ch=", i)
			}
		}(i, ch)

	}

	wg.Wait()

	for _, id := range ids {
		ss.UnregisterStorageChangeChannel(id)
	}
}
