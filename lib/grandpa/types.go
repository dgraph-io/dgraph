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

package grandpa

import (
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto"
)

// Voter struct
//nolint:structcheck
type Voter struct {
	key     crypto.Keypair //nolint:unused
	voterID uint64         //nolint:unused
}

// State struct
//nolint:structcheck
type State struct {
	voters  []Voter //nolint:unused
	counter uint64  //nolint:unused
	round   uint64  //nolint:unused
}

// Vote struct
// nolint:structcheck
type Vote struct {
	hash   common.Hash //nolint:unused
	number uint64      //nolint:unused
}

// VoteMessage struct
//nolint:structcheck
type VoteMessage struct {
	round   uint64   //nolint:unused
	counter uint64   //nolint:unused
	pubkey  [32]byte //nolint:unused // ed25519 public key
	stage   byte     //nolint:unused  // 0 for pre-vote, 1 for pre-commit
}

// Justification struct
//nolint:structcheck
type Justification struct {
	vote      Vote     //nolint:unused
	signature []byte   //nolint:unused
	pubkey    [32]byte //nolint:unused
}

// FinalizationMessage struct
//nolint:structcheck
type FinalizationMessage struct {
	round         uint64        //nolint:unused
	vote          Vote          //nolint:unused
	justification Justification //nolint:unused
}
