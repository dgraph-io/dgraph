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
	"github.com/ChainSafe/gossamer/dot/network"

	log "github.com/ChainSafe/log15"
)

// processConsensusMessage routes a consensus message from the network to the finality gadget
func (s *Service) processConsensusMessage(msg *network.ConsensusMessage) error {
	in := s.finalityGadget.GetVoteInChannel()
	fm, err := s.finalityGadget.DecodeMessage(msg)
	if err != nil {
		return err
	}

	// TODO: safety
	log.Debug("[core] sending VoteMessage to FinalityGadget", "msg", msg)
	in <- fm
	return nil
}

// sendVoteMessages routes a VoteMessage from the finality gadget to the network
func (s *Service) sendVoteMessages() {
	out := s.finalityGadget.GetVoteOutChannel()
	for v := range out {
		// TODO: safety
		msg, err := v.ToConsensusMessage()
		if err != nil {
			log.Error("[core] failed to convert VoteMessage to ConsensusMessage", "msg", msg)
			continue
		}

		log.Debug("[core] sending VoteMessage to network", "msg", msg)
		s.msgSend <- msg
	}
}

// sendFinalityMessages routes a FinalizationMessage from the finality gadget to the network
func (s *Service) sendFinalizationMessages() {
	out := s.finalityGadget.GetFinalizedChannel()
	for v := range out {
		// TODO: safety
		// update state.finalizedHead
		hash, err := v.GetFinalizedHash()
		if err == nil {
			err = s.blockState.SetFinalizedHash(hash)
			if err != nil {
				log.Error("[core] could not set finalized block hash", "hash", hash, "error", err)
			}
		}

		log.Debug("[core] sending FinalityMessage to network", "msg", v)
		log.Info("[core] finalized block!!!", "msg", v)
		msg, err := v.ToConsensusMessage()
		if err != nil {
			log.Error("[core] failed to convert FinalizationMessage to ConsensusMessage", "msg", msg)
			continue
		}

		s.msgSend <- msg
	}
}
