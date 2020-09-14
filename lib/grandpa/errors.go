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
	"errors"

	"github.com/ChainSafe/gossamer/lib/blocktree"
)

// ErrNilBlockState is returned when BlockState is nil
var ErrNilBlockState = errors.New("cannot have nil BlockState")

// ErrNilDigestHandler is returned when DigestHandler is nil
var ErrNilDigestHandler = errors.New("cannot have nil DigestHandler")

// ErrNilKeypair is returned when the keypair is nil
var ErrNilKeypair = errors.New("cannot have nil keypair")

// ErrBlockDoesNotExist is returned when trying to validate a vote for a block that doesn't exist
var ErrBlockDoesNotExist = errors.New("block does not exist")

// ErrInvalidSignature is returned when trying to validate a vote message with an invalid signature
var ErrInvalidSignature = errors.New("signature is not valid")

// ErrSetIDMismatch is returned when trying to validate a vote message with an invalid voter set ID, or when receiving a catch up message with a different set ID
var ErrSetIDMismatch = errors.New("set IDs do not match")

// ErrRoundMismatch is returned when trying to validate a vote message that isn't for the current round
var ErrRoundMismatch = errors.New("rounds do not match")

// ErrEquivocation is returned when trying to validate a vote for that is equivocatory
var ErrEquivocation = errors.New("vote is equivocatory")

// ErrVoterNotFound is returned when trying to validate a vote for a voter that isn't in the voter set
var ErrVoterNotFound = errors.New("voter is not in voter set")

// ErrDescendantNotFound is returned when trying to validate a vote for a block that isn't a descendant of the last finalized block
var ErrDescendantNotFound = blocktree.ErrDescendantNotFound

// ErrNoPreVotedBlock is returned when there is no pre-voted block for a round.
// this can only happen in the case of > 1/3 byzantine nodes (ie > 1/3 nodes equivocate or don't submit valid votes)
var ErrNoPreVotedBlock = errors.New("cannot get pre-voted block")

// ErrNoGHOST is returned when there is no GHOST. the only case where this could happen is if there are no votes
// at all, so it shouldn't ever happen.
var ErrNoGHOST = errors.New("cannot determine grandpa-GHOST")

// ErrCannotDecodeSubround is returned when a subround value cannot be decoded
var ErrCannotDecodeSubround = errors.New("cannot decode invalid subround value")

// ErrInvalidMessageType is returned when a network.Message cannot be decoded
var ErrInvalidMessageType = errors.New("cannot decode invalid message type")

// ErrNotFinalizationMessage is returned when calling GetFinalizedHash on a message that isn't a FinalizationMessage
var ErrNotFinalizationMessage = errors.New("cannot get finalized hash from VoteMessage")

// ErrNoJustification is returned when no justification can be found for a block, ie. it has not been finalized
var ErrNoJustification = errors.New("no justification found for block")

// ErrMinVotesNotMet is returned when the number of votes is less than the required minimum in a Justification
var ErrMinVotesNotMet = errors.New("minimum number of votes not met in a Justification")

// ErrInvalidCatchUpRound is returned when a catch-up message is received with an invalid round
var ErrInvalidCatchUpRound = errors.New("catch up request is for future round")

// ErrInvalidCatchUpResponseRound is returned when a catch-up response is received with an invalid round
var ErrInvalidCatchUpResponseRound = errors.New("catch up response is not for previous round")

// ErrGHOSTlessCatchUp is returned when a catch up response does not contain a valid grandpa-GHOST (ie. finalized block)
var ErrGHOSTlessCatchUp = errors.New("catch up response does not contain grandpa-GHOST")

// ErrCatchUpResponseNotCompletable is returned when the round represented by the catch up response is not completable
var ErrCatchUpResponseNotCompletable = errors.New("catch up response is not completable")

// ErrServicePaused is returned if the service is paused and waiting for catch up messages
var ErrServicePaused = errors.New("service is paused")
