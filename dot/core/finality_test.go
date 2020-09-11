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
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProcessConsensusMessage(t *testing.T) {
	fg := &mockFinalityGadget{
		in:        make(chan FinalityMessage, 2),
		out:       make(chan FinalityMessage, 2),
		finalized: make(chan FinalityMessage, 2),
	}

	s := NewTestService(t, &Config{
		FinalityGadget: fg,
	})
	err := s.processConsensusMessage(testConsensusMessage)
	require.NoError(t, err)
}

func TestSendVoteMessages(t *testing.T) {
	fg := &mockFinalityGadget{
		in:        make(chan FinalityMessage, 2),
		out:       make(chan FinalityMessage, 2),
		finalized: make(chan FinalityMessage, 2),
	}

	net := new(mockNetwork)
	s := NewTestService(t, &Config{
		Network:        net,
		FinalityGadget: fg,
	})

	go s.sendVoteMessages(context.Background())
	fg.out <- &mockFinalityMessage{}

	time.Sleep(testMessageTimeout)
	require.Equal(t, testConsensusMessage, net.Message)
}

func TestSendFinalizationMessages(t *testing.T) {
	fg := &mockFinalityGadget{
		in:        make(chan FinalityMessage, 2),
		out:       make(chan FinalityMessage, 2),
		finalized: make(chan FinalityMessage, 2),
	}

	net := new(mockNetwork)
	s := NewTestService(t, &Config{
		FinalityGadget: fg,
		Network:        net,
	})

	go s.sendFinalizationMessages(context.Background())
	fg.finalized <- &mockFinalityMessage{}

	time.Sleep(testMessageTimeout)
	require.Equal(t, testConsensusMessage, net.Message)
}
