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

package grandpa

import (
	"sync"

	"github.com/ChainSafe/gossamer/lib/common"
)

// tracker keeps track of messages that have been received that have failed to validate with ErrBlockDoesNotExist
// these messages may be needed again in the case that we are slightly out of sync with the rest of the network
type tracker struct {
	messages map[common.Hash][]*VoteMessage // map of vote block hash -> array of VoteMessages for that hash
	mapLock  *sync.Mutex
	in       <-chan common.Hash
	out      chan<- FinalityMessage // send a VoteMessage back to grandpa. corresponds to grandpa's in channel
}

func newTracker(bs BlockState, out chan<- FinalityMessage) *tracker {
	in := make(chan common.Hash)
	bs.SetHashChannel(in)

	return &tracker{
		messages: make(map[common.Hash][]*VoteMessage),
		mapLock:  &sync.Mutex{},
		in:       in,
		out:      out,
	}
}

func (t *tracker) start() {
	go t.handleBlocks()
}

func (t *tracker) stop() {
	// close channel
}

func (t *tracker) add(v *VoteMessage) {
	t.mapLock.Lock()
	t.messages[v.Message.Hash] = append(t.messages[v.Message.Hash], v)
	t.mapLock.Unlock()
}

func (t *tracker) handleBlocks() {
	for h := range t.in {
		t.mapLock.Lock()

		if t.messages[h] != nil {
			for _, v := range t.messages[h] {
				t.out <- v
			}
		}

		t.mapLock.Unlock()
	}
}
