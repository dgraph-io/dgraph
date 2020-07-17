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
	"os"
	"sync"
	"time"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/blocktree"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto/ed25519"

	log "github.com/ChainSafe/log15"
)

var interval = time.Second

// Service represents the current state of the grandpa protocol
type Service struct {
	// preliminaries
	logger     log.Logger
	blockState BlockState
	keypair    *ed25519.Keypair
	mapLock    sync.Mutex
	chanLock   sync.Mutex
	stopped    bool

	// current state information
	state            *State                             // current state
	prevotes         map[ed25519.PublicKeyBytes]*Vote   // pre-votes for the current round
	precommits       map[ed25519.PublicKeyBytes]*Vote   // pre-commits for the current round
	pcJustifications map[common.Hash][]*Justification   // pre-commit justifications for the current round
	pvEquivocations  map[ed25519.PublicKeyBytes][]*Vote // equivocatory votes for current pre-vote stage
	pcEquivocations  map[ed25519.PublicKeyBytes][]*Vote // equivocatory votes for current pre-commit stage
	tracker          *tracker                           // tracker of vote messages we may need in the future
	head             *types.Header                      // most recently finalized block
	nextAuthorities  []*Voter                           // if not nil, the updated authorities for the next round

	// historical information
	preVotedBlock      map[uint64]*Vote            // map of round number -> pre-voted block
	bestFinalCandidate map[uint64]*Vote            // map of round number -> best final candidate
	justification      map[uint64][]*Justification // map of round number -> round justification

	// channels for communication with other services
	in        chan FinalityMessage // only used to receive *VoteMessage
	out       chan FinalityMessage // only used to send *VoteMessage
	finalized chan FinalityMessage // only used to send *FinalizationMessage; channel that finalized blocks are output from at the end of a round
}

// Config represents a GRANDPA service configuration
type Config struct {
	LogLvl     log.Lvl
	BlockState BlockState
	Voters     []*Voter
	SetID      uint64
	Keypair    *ed25519.Keypair
}

// NewService returns a new GRANDPA Service instance.
// TODO: determine what needs to be exported.
func NewService(cfg *Config) (*Service, error) {
	if cfg.BlockState == nil {
		return nil, ErrNilBlockState
	}

	if cfg.Keypair == nil {
		return nil, ErrNilKeypair
	}

	logger := log.New("pkg", "grandpa")
	h := log.StreamHandler(os.Stdout, log.TerminalFormat())
	logger.SetHandler(log.LvlFilterHandler(cfg.LogLvl, h))

	logger.Info("creating service", "key", cfg.Keypair.Public().Hex(), "voter set", Voters(cfg.Voters))

	// get latest finalized header
	head, err := cfg.BlockState.GetFinalizedHeader(0)
	if err != nil {
		return nil, err
	}

	in := make(chan FinalityMessage, 128)

	s := &Service{
		logger:             logger,
		state:              NewState(cfg.Voters, cfg.SetID, 0),
		blockState:         cfg.BlockState,
		keypair:            cfg.Keypair,
		prevotes:           make(map[ed25519.PublicKeyBytes]*Vote),
		precommits:         make(map[ed25519.PublicKeyBytes]*Vote),
		pcJustifications:   make(map[common.Hash][]*Justification),
		pvEquivocations:    make(map[ed25519.PublicKeyBytes][]*Vote),
		pcEquivocations:    make(map[ed25519.PublicKeyBytes][]*Vote),
		preVotedBlock:      make(map[uint64]*Vote),
		bestFinalCandidate: make(map[uint64]*Vote),
		justification:      make(map[uint64][]*Justification),
		head:               head,
		in:                 in,
		out:                make(chan FinalityMessage, 128),
		finalized:          make(chan FinalityMessage, 128),
		stopped:            true,
	}

	return s, nil
}

// Start begins the GRANDPA finality service
func (s *Service) Start() error {
	s.stopped = false

	go func() {
		err := s.initiate()
		if err != nil {
			s.logger.Error("failed to initiate", "error", err)
		}
	}()

	return nil
}

// Stop stops the GRANDPA finality service
func (s *Service) Stop() error {
	s.chanLock.Lock()
	defer s.chanLock.Unlock()

	s.stopped = true
	close(s.out)
	s.tracker.stop()
	return nil
}

// Authorities returns the current grandpa authorities
func (s *Service) Authorities() []*types.GrandpaAuthorityData {
	ad := make([]*types.GrandpaAuthorityData, len(s.state.voters))
	for i, v := range s.state.voters {
		ad[i] = &types.GrandpaAuthorityData{
			Key: v.key,
			ID:  v.id,
		}
	}

	return ad
}

// UpdateAuthorities schedules an update to the grandpa voter set and increments the setID at the end of the current round
func (s *Service) UpdateAuthorities(ad []*types.GrandpaAuthorityData) {
	v := make([]*Voter, len(ad))
	for i, a := range ad {
		v[i] = &Voter{
			key: a.Key,
			id:  a.ID,
		}
	}

	s.nextAuthorities = v
}

// updateAuthorities updates the grandpa voter set, increments the setID, and resets the round numbers
func (s *Service) updateAuthorities() {
	if s.nextAuthorities != nil {
		s.state.voters = s.nextAuthorities
		s.state.setID++
		s.state.round = 0
		s.nextAuthorities = nil
	}
}

func (s *Service) publicKeyBytes() ed25519.PublicKeyBytes {
	return s.keypair.Public().(*ed25519.PublicKey).AsBytes()
}

// initiate initates a GRANDPA round
func (s *Service) initiate() error {
	// if there is an authority change, execute it
	s.updateAuthorities()

	if s.state.round == 0 {
		s.chanLock.Lock()
		s.mapLock.Lock()
		s.preVotedBlock[0] = NewVoteFromHeader(s.head)
		s.bestFinalCandidate[0] = NewVoteFromHeader(s.head)
		s.mapLock.Unlock()
		s.chanLock.Unlock()
	}

	s.state.round++
	if s.tracker != nil {
		s.tracker.stop()
	}

	var err error
	s.prevotes = make(map[ed25519.PublicKeyBytes]*Vote)
	s.precommits = make(map[ed25519.PublicKeyBytes]*Vote)
	s.pcJustifications = make(map[common.Hash][]*Justification)
	s.pvEquivocations = make(map[ed25519.PublicKeyBytes][]*Vote)
	s.pcEquivocations = make(map[ed25519.PublicKeyBytes][]*Vote)
	s.justification = make(map[uint64][]*Justification)
	s.tracker, err = newTracker(s.blockState, s.in)
	if err != nil {
		return err
	}
	s.tracker.start()
	s.logger.Trace("[grandpa] started message tracker")

	// don't begin grandpa until we are at block 1
	for {
		if s.stopped {
			return nil
		}

		h, err := s.blockState.BestBlockHeader()
		if err != nil {
			continue
		}

		if h != nil && h.Number.Int64() > 0 {
			break
		}
	}

	for {
		err := s.playGrandpaRound()
		if err != nil {
			return err
		}

		if s.stopped {
			return nil
		}

		err = s.initiate()
		if err != nil {
			return err
		}
	}
}

// playGrandpaRound executes a round of GRANDPA
// at the end of this round, a block will be finalized.
func (s *Service) playGrandpaRound() error {
	s.logger.Debug("starting round", "round", s.state.round, "setID", s.state.setID)

	// save start time
	start := time.Now()

	// derive primary
	primary := s.derivePrimary()

	// if primary, broadcast the best final candidate from the previous round
	if bytes.Equal(primary.key.Encode(), s.keypair.Public().Encode()) {
		msg := s.newFinalizationMessage(s.head, s.state.round-1)
		s.finalized <- msg
	}

	s.logger.Debug("receiving pre-vote messages...")

	go s.receiveMessages(func() bool {
		end := start.Add(interval * 2)

		completable, err := s.isCompletable()
		if err != nil {
			// ignore, since if round isn't completable then this will continue
		}

		if time.Since(end) >= 0 || completable {
			return true
		}

		return false
	})

	time.Sleep(interval * 2)

	// broadcast pre-vote
	pv, err := s.determinePreVote()
	if err != nil {
		return err
	}

	s.mapLock.Lock()
	s.prevotes[s.publicKeyBytes()] = pv
	s.logger.Debug("sending pre-vote message...", "vote", pv, "prevotes", s.prevotes)
	s.mapLock.Unlock()

	finalized := false

	// continue to send prevote messages until round is done
	go func(finalized *bool) {
		for {
			if *finalized {
				return
			}

			err = s.sendMessage(pv, prevote)
			if err != nil {
				s.logger.Error("could not send prevote message", "error", err)
			}

			time.Sleep(time.Second * 5)
			s.logger.Trace("sent pre-vote message...", "vote", pv, "prevotes", s.prevotes)
		}
	}(&finalized)

	s.logger.Debug("receiving pre-vote messages...")

	go s.receiveMessages(func() bool {
		end := start.Add(interval * 4)

		completable, err := s.isCompletable() //nolint
		if err != nil {
			// ignore, since if round isn't completable then this will continue
		}

		if time.Since(end) >= 0 || completable {
			return true
		}

		return false
	})

	time.Sleep(interval * 2)

	// broadcast pre-commit
	pc, err := s.determinePreCommit()
	if err != nil {
		return err
	}

	s.mapLock.Lock()
	s.precommits[s.publicKeyBytes()] = pc
	s.logger.Debug("sending pre-commit message...", "vote", pc, "precommits", s.precommits)
	s.mapLock.Unlock()

	// continue to send precommit messages until round is done
	go func(finalized *bool) {
		for {
			if *finalized {
				return
			}

			err = s.sendMessage(pc, precommit)
			if err != nil {
				s.logger.Error("could not send precommit message", "error", err)
			}

			time.Sleep(time.Second * 5)
			s.logger.Trace("sent pre-commit message...", "vote", pc, "precommits", s.precommits)
		}
	}(&finalized)

	go func() {
		// receive messages until current round is completable and previous round is finalizable
		// and the last finalized block is greater than the best final candidate from the previous round
		s.receiveMessages(func() bool {
			completable, err := s.isCompletable() //nolint
			if err != nil {
				return false
			}

			finalizable, err := s.isFinalizable(s.state.round)
			if err != nil {
				return false
			}

			s.mapLock.Lock()
			prevBfc := s.bestFinalCandidate[s.state.round-1]
			s.mapLock.Unlock()

			// this shouldn't happen as long as playGrandpaRound is called through initiate
			if prevBfc == nil {
				return false
			}

			if completable && finalizable && uint64(s.head.Number.Int64()) >= prevBfc.number {
				return true
			}

			return false
		})
	}()

	err = s.attemptToFinalize()
	if err != nil {
		log.Error("[grandpa] failed to finalize", "error", err)
		return err
	}

	finalized = true
	return nil
}

// attemptToFinalize loops until the round is finalizable
func (s *Service) attemptToFinalize() error {
	bfc, err := s.getBestFinalCandidate()
	if err != nil {
		return err
	}

	pc, err := s.getTotalVotesForBlock(bfc.hash, precommit)
	if err != nil {
		return err
	}

	if bfc.number >= uint64(s.head.Number.Int64()) && pc >= s.state.threshold() {
		err = s.finalize()
		if err != nil {
			return err
		}

		// if we haven't received a finalization message for this block yet, broadcast a finalization message
		s.logger.Debug("finalized block!!!", "round", s.state.round, "hash", s.head.Hash())
		msg := s.newFinalizationMessage(s.head, s.state.round)

		// TODO: safety
		s.finalized <- msg
		return nil
	}

	time.Sleep(time.Millisecond * 10)
	return s.attemptToFinalize()
}

// determinePreVote determines what block is our pre-voted block for the current round
func (s *Service) determinePreVote() (*Vote, error) {
	var vote *Vote

	// if we receive a vote message from the primary with a block that's greater than or equal to the current pre-voted block
	// and greater than the best final candidate from the last round, we choose that.
	// otherwise, we simply choose the head of our chain.
	s.mapLock.Lock()
	prm := s.prevotes[s.derivePrimary().PublicKeyBytes()]
	s.mapLock.Unlock()

	if prm != nil && prm.number >= uint64(s.head.Number.Int64()) {
		vote = prm
	} else {
		header, err := s.blockState.BestBlockHeader()
		if err != nil {
			return nil, err
		}

		vote = NewVoteFromHeader(header)
	}

	return vote, nil
}

// determinePreCommit determines what block is our pre-committed block for the current round
func (s *Service) determinePreCommit() (*Vote, error) {
	// the pre-committed block is simply the pre-voted block (GRANDPA-GHOST)
	pvb, err := s.getPreVotedBlock()
	if err != nil {
		return nil, err
	}

	return &pvb, nil
}

// isFinalizable returns true is the round is finalizable, false otherwise.
func (s *Service) isFinalizable(round uint64) (bool, error) {
	var pvb Vote
	var err error

	if round == 0 {
		return true, nil
	}

	if round == s.state.round {
		pvb, err = s.getPreVotedBlock()
		if err != nil {
			return false, err
		}
	} else {
		s.mapLock.Lock()
		v, has := s.preVotedBlock[round]
		s.mapLock.Unlock()

		if !has {
			return false, ErrNoPreVotedBlock
		}
		pvb = *v
	}

	bfc, err := s.getBestFinalCandidate()
	if err != nil {
		return false, err
	}

	pc, err := s.getTotalVotesForBlock(bfc.hash, precommit)
	if err != nil {
		return false, err
	}

	s.mapLock.Lock()
	prevBfc := s.bestFinalCandidate[s.state.round-1]
	s.mapLock.Unlock()

	if bfc.number <= pvb.number && (s.state.round == 0 || prevBfc.number <= bfc.number) && pc >= s.state.threshold() {
		return true, nil
	}

	return false, nil
}

// finalize finalizes the round by setting the best final candidate for this round
func (s *Service) finalize() error {
	// get best final candidate
	bfc, err := s.getBestFinalCandidate()
	if err != nil {
		return err
	}

	pv, err := s.getPreVotedBlock()
	if err != nil {
		return err
	}
	s.mapLock.Lock()
	s.preVotedBlock[s.state.round] = &pv

	// set best final candidate
	s.bestFinalCandidate[s.state.round] = bfc

	// set justification
	s.justification[s.state.round] = s.pcJustifications[bfc.hash]
	s.mapLock.Unlock()

	s.head, err = s.blockState.GetHeader(bfc.hash)
	if err != nil {
		return err
	}

	// set finalized head for round in db
	err = s.blockState.SetFinalizedHash(bfc.hash, s.state.round)
	if err != nil {
		return err
	}

	// set latest finalized head in db
	return s.blockState.SetFinalizedHash(bfc.hash, 0)
}

// derivePrimary returns the primary for the current round
func (s *Service) derivePrimary() *Voter {
	return s.state.voters[s.state.round%uint64(len(s.state.voters))]
}

// getBestFinalCandidate calculates the set of blocks that are less than or equal to the pre-voted block in height,
// with >= 2/3 pre-commit votes, then returns the block with the highest number from this set.
func (s *Service) getBestFinalCandidate() (*Vote, error) {
	prevoted, err := s.getPreVotedBlock()
	if err != nil {
		return nil, err
	}

	// get all blocks with >=2/3 pre-commits
	blocks, err := s.getPossibleSelectedBlocks(precommit, s.state.threshold())
	if err != nil {
		return nil, err
	}

	// if there are no blocks with >=2/3 pre-commits, just return the pre-voted block
	// TODO: is this correct? the spec implies that it should return nil, but discussions have suggested
	// that we return the prevoted block.
	if len(blocks) == 0 {
		return &prevoted, nil
	}

	// if there are multiple blocks, get the one with the highest number
	// that is also an ancestor of the prevoted block (or is the prevoted block)
	if blocks[prevoted.hash] != 0 {
		return &prevoted, nil
	}

	bfc := &Vote{
		number: 0,
	}

	for h, n := range blocks {
		// check if the current block is an ancestor of prevoted block
		isDescendant, err := s.blockState.IsDescendantOf(h, prevoted.hash)
		if err != nil {
			return nil, err
		}

		if !isDescendant {
			// find common ancestor, implicitly has >=2/3 votes
			pred, err := s.blockState.HighestCommonAncestor(h, prevoted.hash)
			if err != nil {
				return nil, err
			}

			v, err := NewVoteFromHash(pred, s.blockState)
			if err != nil {
				return nil, err
			}

			n = v.number
			h = pred
		}

		// choose block with highest number
		if n > bfc.number {
			bfc = &Vote{
				hash:   h,
				number: n,
			}
		}
	}

	if [32]byte(bfc.hash) == [32]byte{} {
		return &prevoted, nil
	}

	return bfc, nil
}

// isCompletable returns true if the round is completable, false otherwise
func (s *Service) isCompletable() (bool, error) {
	votes := s.getVotes(precommit)
	prevoted, err := s.getPreVotedBlock()
	if err != nil {
		return false, err
	}

	for _, v := range votes {
		if prevoted.hash == v.hash {
			continue
		}

		// check if the current block is a descendant of prevoted block
		isDescendant, err := s.blockState.IsDescendantOf(prevoted.hash, v.hash)
		if err != nil {
			return false, err
		}

		if !isDescendant {
			continue
		}

		// if it's a descendant, check if has >=2/3 votes
		c, err := s.getTotalVotesForBlock(v.hash, precommit)
		if err != nil {
			return false, err
		}

		if c > s.state.threshold() {
			// round isn't completable
			return false, nil
		}
	}

	return true, nil
}

// getPreVotedBlock returns the current pre-voted block B. also known as GRANDPA-GHOST.
// the pre-voted block is the block with the highest block number in the set of all the blocks with
// total votes >= 2/3 the total number of voters, where the total votes is determined by getTotalVotesForBlock.
func (s *Service) getPreVotedBlock() (Vote, error) {
	blocks, err := s.getPossibleSelectedBlocks(prevote, s.state.threshold())
	if err != nil {
		return Vote{}, err
	}

	// TODO: if there are no blocks with >=2/3 voters, then just pick the highest voted block
	if len(blocks) == 0 {
		return s.getGrandpaGHOST()
	}

	// if there is one block, return it
	if len(blocks) == 1 {
		for h, n := range blocks {
			return Vote{
				hash:   h,
				number: n,
			}, nil
		}
	}

	// if there are multiple, find the one with the highest number and return it
	highest := Vote{
		number: uint64(0),
	}
	for h, n := range blocks {
		if n > highest.number {
			highest = Vote{
				hash:   h,
				number: n,
			}
		}
	}

	return highest, nil
}

// getGrandpaGHOST returns the block with the most votes. if there are multiple blocks with the same number
// of votes, it picks the one with the highest number.
func (s *Service) getGrandpaGHOST() (Vote, error) {
	threshold := s.state.threshold()

	var blocks map[common.Hash]uint64
	var err error

	for {
		blocks, err = s.getPossibleSelectedBlocks(prevote, threshold)
		if err != nil {
			return Vote{}, err
		}

		threshold--
		if len(blocks) > 0 || threshold == 0 {
			break
		}
	}

	if len(blocks) == 0 {
		return Vote{}, ErrNoGHOST
	}

	// if there are multiple, find the one with the highest number and return it
	highest := Vote{
		number: uint64(0),
	}
	for h, n := range blocks {
		if n > highest.number {
			highest = Vote{
				hash:   h,
				number: n,
			}
		}
	}

	return highest, nil
}

// getPossibleSelectedBlocks returns blocks with total votes >=threshold in a map of block hash -> block number.
// if there are no blocks that have >=threshold direct votes, this function will find ancestors of those blocks that do have >=threshold votes.
// note that by voting for a block, all of its ancestor blocks are automatically voted for.
// thus, if there are no blocks with >=threshold total votes, but the sum of votes for blocks A and B is >=threshold, then this function returns
// the first common ancestor of A and B.
// in general, this function will return the highest block on each chain with >=threshold votes.
func (s *Service) getPossibleSelectedBlocks(stage subround, threshold uint64) (map[common.Hash]uint64, error) {
	// get blocks that were directly voted for
	votes := s.getDirectVotes(stage)
	blocks := make(map[common.Hash]uint64)

	// check if any of them have >=threshold votes
	for v := range votes {
		total, err := s.getTotalVotesForBlock(v.hash, stage)
		if err != nil {
			return nil, err
		}

		if total >= threshold {
			blocks[v.hash] = v.number
		}
	}

	// since we want to select the block with the highest number that has >=threshold votes,
	// we can return here since their ancestors won't have a higher number.
	if len(blocks) != 0 {
		return blocks, nil
	}

	// no block has >=threshold direct votes, check for votes for ancestors recursively
	var err error
	va := s.getVotes(stage)

	for v := range votes {
		blocks, err = s.getPossibleSelectedAncestors(va, v.hash, blocks, stage, threshold)
		if err != nil {
			return nil, err
		}
	}

	return blocks, nil
}

// getPossibleSelectedAncestors recursively searches for ancestors with >=2/3 votes
// it returns a map of block hash -> number, such that the blocks in the map have >=2/3 votes
func (s *Service) getPossibleSelectedAncestors(votes []Vote, curr common.Hash, selected map[common.Hash]uint64, stage subround, threshold uint64) (map[common.Hash]uint64, error) {
	for _, v := range votes {
		if v.hash == curr {
			continue
		}

		// find common ancestor, check if votes for it is >=threshold or not
		pred, err := s.blockState.HighestCommonAncestor(v.hash, curr)
		if err == blocktree.ErrNodeNotFound {
			continue
		} else if err != nil {
			return nil, err
		}

		if pred == curr {
			return selected, nil
		}

		total, err := s.getTotalVotesForBlock(pred, stage)
		if err != nil {
			return nil, err
		}

		if total >= threshold {
			var h *types.Header
			h, err = s.blockState.GetHeader(pred)
			if err != nil {
				return nil, err
			}

			selected[pred] = uint64(h.Number.Int64())
		} else {
			selected, err = s.getPossibleSelectedAncestors(votes, pred, selected, stage, threshold)
			if err != nil {
				return nil, err
			}
		}
	}

	return selected, nil
}

// getTotalVotesForBlock returns the total number of observed votes for a block B in a subround, which is equal
// to the direct votes for B and B's descendants plus the total number of equivocating voters
func (s *Service) getTotalVotesForBlock(hash common.Hash, stage subround) (uint64, error) {
	// observed votes for block
	dv, err := s.getVotesForBlock(hash, stage)
	if err != nil {
		return 0, err
	}

	// equivocatory votes
	var ev int
	if stage == prevote {
		ev = len(s.pvEquivocations)
	} else {
		ev = len(s.pcEquivocations)
	}

	return dv + uint64(ev), nil
}

// getVotesForBlock returns the number of observed votes for a block B.
// The set of all observed votes by v in the sub-round stage of round r for block B is
// equal to all of the observed direct votes cast for block B and all of the B's descendants
func (s *Service) getVotesForBlock(hash common.Hash, stage subround) (uint64, error) {
	votes := s.getDirectVotes(stage)

	// B will be counted as in it's own subchain, so don't need to start with B's vote count
	votesForBlock := uint64(0)

	for v, c := range votes {

		// check if the current block is a descendant of B
		isDescendant, err := s.blockState.IsDescendantOf(hash, v.hash)
		if err == blocktree.ErrStartNodeNotFound || err == blocktree.ErrEndNodeNotFound {
			continue
		} else if err != nil {
			return 0, err
		}

		if !isDescendant {
			continue
		}

		votesForBlock += c
	}

	return votesForBlock, nil
}

// getDirectVotes returns a map of Votes to direct vote counts
func (s *Service) getDirectVotes(stage subround) map[Vote]uint64 {
	votes := make(map[Vote]uint64)

	var src map[ed25519.PublicKeyBytes]*Vote
	if stage == prevote {
		src = s.prevotes
	} else {
		src = s.precommits
	}

	s.mapLock.Lock()
	defer s.mapLock.Unlock()

	for _, v := range src {
		votes[*v]++
	}

	return votes
}

// getVotes returns all the current votes as an array
func (s *Service) getVotes(stage subround) []Vote {
	votes := s.getDirectVotes(stage)
	va := make([]Vote, len(votes))
	i := 0

	for v := range votes {
		va[i] = v
		i++
	}

	return va
}

// findParentWithNumber returns a Vote for an ancestor with number n given an existing Vote
func (s *Service) findParentWithNumber(v *Vote, n uint64) (*Vote, error) {
	if v.number <= n {
		return v, nil
	}

	b, err := s.blockState.GetHeader(v.hash)
	if err != nil {
		return nil, err
	}

	// # of iterations
	l := int(v.number - n)

	for i := 0; i < l; i++ {
		p, err := s.blockState.GetHeader(b.ParentHash)
		if err != nil {
			return nil, err
		}

		b = p
	}

	return NewVoteFromHeader(b), nil
}
