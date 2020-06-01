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
	"bytes"
	"fmt"

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

// PublicKeyBytes returns the voter key as PublicKeyBytes
func (v *Voter) PublicKeyBytes() ed25519.PublicKeyBytes {
	return v.key.AsBytes()
}

// State represents a GRANDPA state
type State struct {
	voters []*Voter // set of voters
	setID  uint64   // authority set ID
	round  uint64   // voting round number
}

// NewState returns a new GRANDPA state
func NewState(voters []*Voter, setID, round uint64) *State {
	return &State{
		voters: voters,
		setID:  setID,
		round:  round,
	}
}

// pubkeyToVoter returns a Voter given a public key
func (s *State) pubkeyToVoter(pk *ed25519.PublicKey) (*Voter, error) {
	max := uint64(2^64) - 1
	id := max

	for i, v := range s.voters {
		if bytes.Equal(pk.Encode(), v.key.Encode()) {
			id = uint64(i)
			break
		}
	}

	if id == max {
		return nil, ErrVoterNotFound
	}

	return &Voter{
		key: pk,
		id:  id,
	}, nil
}

// threshold returns the 2/3 |voters| threshold value
// TODO: determine rounding, is currently set to floor
func (s *State) threshold() uint64 {
	return uint64(2 * len(s.voters) / 3)
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

// NewVoteFromHash returns a new Vote given a hash and a blockState
func NewVoteFromHash(hash common.Hash, blockState BlockState) (*Vote, error) {
	has, err := blockState.HasHeader(hash)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, ErrBlockDoesNotExist
	}

	h, err := blockState.GetHeader(hash)
	if err != nil {
		return nil, err
	}

	return NewVoteFromHeader(h), nil
}

func (v *Vote) String() string {
	return fmt.Sprintf("hash=%s number=%d", v.hash, v.number)
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
