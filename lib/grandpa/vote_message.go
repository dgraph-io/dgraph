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
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/ChainSafe/gossamer/lib/crypto"
	"github.com/ChainSafe/gossamer/lib/crypto/ed25519"
	"github.com/ChainSafe/gossamer/lib/scale"
)

// receiveMessages receives messages from the in channel until the specified condition is met
func (s *Service) receiveMessages(cond func() bool) {
	ctx, cancel := context.WithCancel(s.ctx)

	go func() {
		for {
			select {
			case msg := <-s.in:
				if msg == nil {
					continue
				}

				s.logger.Trace("received vote message", "msg", msg)
				vm, ok := msg.(*VoteMessage)
				if !ok {
					s.logger.Trace("failed to cast message to VoteMessage")
					continue
				}

				v, err := s.validateMessage(vm)
				if err != nil {
					s.logger.Trace("failed to validate vote message", "message", vm, "error", err)
					continue
				}

				s.logger.Debug("validated vote message", "vote", v, "round", vm.Round, "subround", vm.Stage, "precommits", s.precommits)
			case <-ctx.Done():
				s.logger.Trace("returning from receiveMessages")
				return
			}
		}
	}()

	for {
		if cond() {
			cancel()
			return
		}
		time.Sleep(time.Millisecond * 10)
	}
}

// sendMessage sends a message through the out channel
func (s *Service) sendMessage(vote *Vote, stage subround) error {
	msg, err := s.createVoteMessage(vote, stage, s.keypair)
	if err != nil {
		return err
	}

	s.chanLock.Lock()
	defer s.chanLock.Unlock()

	// context was canceled
	if s.ctx.Err() != nil {
		return nil
	}

	s.out <- msg
	s.logger.Trace("sent VoteMessage", "msg", msg)

	return nil
}

// createVoteMessage returns a signed VoteMessage given a header
func (s *Service) createVoteMessage(vote *Vote, stage subround, kp crypto.Keypair) (*VoteMessage, error) {
	msg, err := scale.Encode(&FullVote{
		Stage: stage,
		Vote:  vote,
		Round: s.state.round,
		SetID: s.state.setID,
	})
	if err != nil {
		return nil, err
	}

	sig, err := kp.Sign(msg)
	if err != nil {
		return nil, err
	}

	sm := &SignedMessage{
		Hash:        vote.hash,
		Number:      vote.number,
		Signature:   ed25519.NewSignatureBytes(sig),
		AuthorityID: kp.Public().(*ed25519.PublicKey).AsBytes(),
	}

	return &VoteMessage{
		Round:   s.state.round,
		SetID:   s.state.setID,
		Stage:   stage,
		Message: sm,
	}, nil
}

// validateMessage validates a VoteMessage and adds it to the current votes
// it returns the resulting vote if validated, error otherwise
func (s *Service) validateMessage(m *VoteMessage) (*Vote, error) {
	// make sure round does not increment while VoteMessage is being validated
	s.roundLock.Lock()
	defer s.roundLock.Unlock()

	if m.Message == nil {
		return nil, errors.New("invalid VoteMessage; missing Message field")
	}

	// check for message signature
	pk, err := ed25519.NewPublicKey(m.Message.AuthorityID[:])
	if err != nil {
		return nil, err
	}

	err = validateMessageSignature(pk, m)
	if err != nil {
		return nil, err
	}

	// check that setIDs match
	if m.SetID != s.state.setID {
		return nil, ErrSetIDMismatch
	}

	// check that vote is for current round
	if m.Round != s.state.round {
		return nil, ErrRoundMismatch
	}

	// check for equivocation ie. multiple votes within one subround
	voter, err := s.state.pubkeyToVoter(pk)
	if err != nil {
		return nil, err
	}

	vote := NewVote(m.Message.Hash, m.Message.Number)

	// if the vote is from ourselves, ignore
	kb := [32]byte(s.publicKeyBytes())
	if bytes.Equal(m.Message.AuthorityID[:], kb[:]) {
		return vote, nil
	}

	err = s.validateVote(vote)
	if err == ErrBlockDoesNotExist {
		s.tracker.add(m)
	}
	if err != nil {
		return nil, err
	}

	s.mapLock.Lock()
	defer s.mapLock.Unlock()

	just := &Justification{
		Vote:        vote,
		Signature:   m.Message.Signature,
		AuthorityID: pk.AsBytes(),
	}

	// add justification before checking for equivocation, since equivocatory vote may still be used in justification
	if m.Stage == prevote {
		s.pvJustifications[m.Message.Hash] = append(s.pvJustifications[m.Message.Hash], just)
	} else if m.Stage == precommit {
		s.pcJustifications[m.Message.Hash] = append(s.pcJustifications[m.Message.Hash], just)
	}

	equivocated := s.checkForEquivocation(voter, vote, m.Stage)
	if equivocated {
		return nil, ErrEquivocation
	}

	if m.Stage == prevote {
		s.prevotes[pk.AsBytes()] = vote
	} else if m.Stage == precommit {
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

	if votes[v] != nil && votes[v].hash != vote.hash {
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
		Stage: m.Stage,
		Vote:  NewVote(m.Message.Hash, m.Message.Number),
		Round: m.Round,
		SetID: m.SetID,
	})
	if err != nil {
		return err
	}

	ok, err := pk.Verify(msg, m.Message.Signature[:])
	if err != nil {
		return err
	}

	if !ok {
		return ErrInvalidSignature
	}

	return nil
}
