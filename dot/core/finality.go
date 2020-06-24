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
)

// processConsensusMessage routes a consensus message from the network to the finality gadget
func (s *Service) processConsensusMessage(msg *network.ConsensusMessage) error {
	in := s.finalityGadget.GetVoteInChannel()
	fm, err := s.finalityGadget.DecodeMessage(msg)
	if err != nil {
		return err
	}

	// TODO: safety
	s.logger.Debug("sending VoteMessage to FinalityGadget", "msg", msg)
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
			s.logger.Error("failed to convert VoteMessage to ConsensusMessage", "msg", msg)
			continue
		}

		s.logger.Debug("sending VoteMessage to network", "msg", msg)
		s.msgSend <- msg
	}
}

// sendFinalityMessages routes a FinalizationMessage from the finality gadget to the network
func (s *Service) sendFinalizationMessages() {
	out := s.finalityGadget.GetFinalizedChannel()
	for v := range out {
		s.logger.Info("finalized block!!!", "msg", v)
		msg, err := v.ToConsensusMessage()
		if err != nil {
			s.logger.Error("failed to convert FinalizationMessage to ConsensusMessage", "msg", msg)
			continue
		}

		// update finalized hash for this round in database
		// TODO: this also happens in grandpa.finalize(); decide which is preferred
		hash, err := v.GetFinalizedHash()
		if err == nil {
			err = s.blockState.SetFinalizedHash(hash, v.GetRound())
			if err != nil {
				s.logger.Error("could not set finalized block hash", "hash", hash, "error", err)
			}

			err = s.blockState.SetFinalizedHash(hash, 0)
			if err != nil {
				s.logger.Error("could not set finalized block hash", "hash", hash, "error", err)
			}
		}

		s.logger.Debug("sending FinalityMessage to network", "msg", v)
		err = s.safeMsgSend(msg)
		if err != nil {
			s.logger.Error("failed to send finalization message to network", "error", err)
		}
	}
}
