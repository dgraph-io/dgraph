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

package sync

import (
	"github.com/ChainSafe/gossamer/dot/types"
)

var firstEpochInfo = &types.EpochInfo{
	Duration:   200,
	FirstBlock: 0,
}

// mockVerifier implements the Verifier interface
type mockVerifier struct{}

// VerifyBlock mocks verifying a block
func (v *mockVerifier) VerifyBlock(header *types.Header) (bool, error) {
	return true, nil
}

// mockBlockProducer implements the BlockProducer interface
type mockBlockProducer struct {
	auths []*types.Authority
}

func newMockBlockProducer() *mockBlockProducer {
	return &mockBlockProducer{
		auths: []*types.Authority{},
	}
}

// Pause mocks pausing
func (bp *mockBlockProducer) Pause() error {
	return nil
}

// Resume mocks resuming
func (bp *mockBlockProducer) Resume() error {
	return nil
}
