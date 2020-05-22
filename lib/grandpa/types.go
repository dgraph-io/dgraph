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
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto/ed25519"
)

type subround = byte

var prevote subround = 0
var precommit subround = 1 //nolint

// Voter represents a GRANDPA voter
type Voter struct {
	key *ed25519.PublicKey
	id  uint64 //nolint:unused
}

// State represents a GRANDPA state
type State struct {
	voters []*Voter // set of voters
	setID  uint64   // authority set ID
	round  uint64   // voting round number
}

// Vote represents a vote for a block with the given hash and number
type Vote struct {
	hash   common.Hash
	number uint64
}

// NewVote returns a new Vote given a block hash and number
func NewVote(hash common.Hash, number uint64) *Vote {
	return &Vote{
		hash:   hash,
		number: number,
	}
}

// NewVoteFromHeader returns a new Vote for a given header
func NewVoteFromHeader(h *types.Header) *Vote {
	return &Vote{
		hash:   h.Hash(),
		number: uint64(h.Number.Int64()),
	}
}

// FullVote represents a vote with additional information about the state
// this is encoded and signed and the signature is included in SignedMessage
type FullVote struct {
	stage byte
	vote  *Vote
	round uint64
	setID uint64
}

// VoteMessage represents a network-level vote message
// https://github.com/paritytech/substrate/blob/master/client/finality-grandpa/src/communication/gossip.rs#L336
type VoteMessage struct {
	setID   uint64
	round   uint64
	stage   byte // 0 for pre-vote, 1 for pre-commit
	message *SignedMessage
}

// SignedMessage represents a block hash and number signed by an authority
// https://github.com/paritytech/substrate/blob/master/client/finality-grandpa/src/lib.rs#L146
type SignedMessage struct {
	hash        common.Hash
	number      uint64
	signature   [64]byte // ed25519.SignatureLength
	authorityID ed25519.PublicKeyBytes
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
