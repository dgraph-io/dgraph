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
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/crypto"
	"github.com/ChainSafe/gossamer/lib/crypto/ed25519"
	"github.com/ChainSafe/gossamer/lib/scale"
)

// CreateVoteMessage returns a signed VoteMessage given a header
func (s *Service) CreateVoteMessage(header *types.Header, kp crypto.Keypair) (*VoteMessage, error) {
	vote := NewVoteFromHeader(header)

	msg, err := scale.Encode(&FullVote{
		stage: s.subround,
		vote:  vote,
		round: s.state.round,
		setID: s.state.setID,
	})
	if err != nil {
		return nil, err
	}

	sig, err := kp.Sign(msg)
	if err != nil {
		return nil, err
	}

	sm := &SignedMessage{
		hash:        vote.hash,
		number:      vote.number,
		signature:   ed25519.NewSignatureBytes(sig),
		authorityID: kp.Public().(*ed25519.PublicKey).AsBytes(),
	}

	return &VoteMessage{
		setID:   s.state.setID,
		round:   s.state.round,
		stage:   s.subround,
		message: sm,
	}, nil
}

// ValidateMessage validates a VoteMessage and adds it to the current votes
// it returns the resulting vote if validated, error otherwise
func (s *Service) ValidateMessage(m *VoteMessage) (*Vote, error) {
	// check for message signature
	pk, err := ed25519.NewPublicKey(m.message.authorityID[:])
	if err != nil {
		return nil, err
	}

	err = validateMessageSignature(pk, m)
	if err != nil {
		return nil, err
	}

	// check that setIDs match
	if m.setID != s.state.setID {
		return nil, ErrSetIDMismatch
	}

	// check for equivocation ie. multiple votes within one subround
	voter, err := s.state.pubkeyToVoter(pk)
	if err != nil {
		return nil, err
	}

	vote := NewVote(m.message.hash, m.message.number)

	equivocated := s.checkForEquivocation(voter, vote, m.stage)
	if equivocated {
		return nil, ErrEquivocation
	}

	err = s.validateVote(vote)
	if err != nil {
		return nil, err
	}

	if m.stage == prevote {
		s.prevotes[pk.AsBytes()] = vote
	} else if m.stage == precommit {
		s.precommits[pk.AsBytes()] = vote
	}

	return vote, nil
}

// checkForEquivocation checks if the vote is an equivocatory vote.
// it returns true if so, false otherwise.
// additionally, if the vote is equivocatory, it updates the service's votes and equivocations.
func (s *Service) checkForEquivocation(voter *Voter, vote *Vote, stage subround) bool {
	v := voter.key.AsBytes()

	var eq map[ed25519.PublicKeyBytes][]*Vote
	var votes map[ed25519.PublicKeyBytes]*Vote

	if stage == prevote {
		eq = s.pvEquivocations
		votes = s.prevotes
	} else {
		eq = s.pcEquivocations
		votes = s.precommits
	}

	if eq[v] != nil {
		// if the voter has already equivocated, every vote in that round is an equivocatory vote
		eq[v] = append(eq[v], vote)
		return true
	}

	if votes[v] != nil {
		// the voter has already voted, all their votes are now equivocatory
		prev := votes[v]
		eq[v] = []*Vote{prev, vote}
		delete(votes, v)
		return true
	}

	return false
}

// validateVote checks if the block that is being voted for exists, and that it is a descendant of a
// previously finalized block.
func (s *Service) validateVote(v *Vote) error {
	// check if v.hash corresponds to a valid block
	has, err := s.blockState.HasHeader(v.hash)
	if err != nil {
		return err
	}

	if !has {
		return ErrBlockDoesNotExist
	}

	// check if the block is an eventual descendant of a previously finalized block
	isDescendant, err := s.blockState.IsDescendantOf(s.head.Hash(), v.hash)
	if err != nil {
		return err
	}

	if !isDescendant {
		return ErrDescendantNotFound
	}

	return nil
}

func validateMessageSignature(pk *ed25519.PublicKey, m *VoteMessage) error {
	msg, err := scale.Encode(&FullVote{
		stage: m.stage,
		vote:  NewVote(m.message.hash, m.message.number),
		round: m.round,
		setID: m.setID,
	})
	if err != nil {
		return err
	}
	ok, err := pk.Verify(msg, m.message.signature[:])
	if err != nil {
		return err
	}

	if !ok {
		return ErrInvalidSignature
	}

	return nil
}
