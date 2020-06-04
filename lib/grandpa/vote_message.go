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
	"time"

	"github.com/ChainSafe/gossamer/lib/crypto"
	"github.com/ChainSafe/gossamer/lib/crypto/ed25519"
	"github.com/ChainSafe/gossamer/lib/scale"

	log "github.com/ChainSafe/log15"
)

// sendMessageTimeout is the timeout period for sending a Vote through the out channel
var sendMessageTimeout = time.Second

// receiveMessages receives messages from the in channel until the specified condition is met
func (s *Service) receiveMessages(cond func() bool) {
	go func() {
		for msg := range s.in {
			log.Debug("[grandpa] received vote message", "msg", msg)

			v, err := s.validateMessage(msg)
			if err != nil {
				log.Debug("[grandpa] failed to validate vote message", "message", msg, "error", err)
				continue
			}

			log.Debug("[grandpa] validated vote message", "vote", v, "subround", msg.stage)
		}
	}()

	for {
		if cond() {
			return
		}
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

	if s.stopped {
		return nil
	}

	select {
	case s.out <- msg:
	case <-time.After(sendMessageTimeout):
		log.Warn("[grandpa] failed to send vote message", "vote", vote)
		return nil
	}

	return nil
}

// createVoteMessage returns a signed VoteMessage given a header
func (s *Service) createVoteMessage(vote *Vote, stage subround, kp crypto.Keypair) (*VoteMessage, error) {
	msg, err := scale.Encode(&FullVote{
		stage: stage,
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
		stage:   stage,
		message: sm,
	}, nil
}

// validateMessage validates a VoteMessage and adds it to the current votes
// it returns the resulting vote if validated, error otherwise
func (s *Service) validateMessage(m *VoteMessage) (*Vote, error) {
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

	// if the vote is from ourselves, ignore
	kb := [32]byte(s.publicKeyBytes())
	if bytes.Equal(m.message.authorityID[:], kb[:]) {
		return vote, nil
	}

	equivocated := s.checkForEquivocation(voter, vote, m.stage)
	if equivocated {
		return nil, ErrEquivocation
	}

	err = s.validateVote(vote)
	if err != nil {
		return nil, err
	}

	s.mapLock.Lock()
	defer s.mapLock.Unlock()

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

	s.mapLock.Lock()
	defer s.mapLock.Unlock()

	if stage == prevote {
		eq = s.pvEquivocations
		votes = s.prevotes
	} else {
		eq = s.pcEquivocations
		votes = s.precommits
	}

	// TODO: check for votes we've already seen #923
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
