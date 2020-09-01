package grandpa

import (
	"bytes"
	"fmt"

	"github.com/ChainSafe/gossamer/dot/core"
	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto/ed25519"
	"github.com/ChainSafe/gossamer/lib/scale"
)

// FinalityMessage is an alias for the core.FinalityMessage interface
type FinalityMessage = core.FinalityMessage

// ConsensusMessage is an alias for network.ConsensusMessage
type ConsensusMessage = network.ConsensusMessage

// GetVoteOutChannel returns a read-only VoteMessage channel
func (s *Service) GetVoteOutChannel() <-chan FinalityMessage {
	return s.out
}

// GetVoteInChannel returns a write-only VoteMessage channel
func (s *Service) GetVoteInChannel() chan<- FinalityMessage {
	return s.in
}

// GetFinalizedChannel returns a read-only FinalizationMessage channel
func (s *Service) GetFinalizedChannel() <-chan FinalityMessage {
	return s.finalized
}

var (
	voteType            byte = 0
	precommitType       byte = 1
	finalizationType    byte = 2
	catchUpRequestType  byte = 3
	catchUpResponseType byte = 4
)

// FullVote represents a vote with additional information about the state
// this is encoded and signed and the signature is included in SignedMessage
type FullVote struct {
	Stage subround
	Vote  *Vote
	Round uint64
	SetID uint64
}

// SignedMessage represents a block hash and number signed by an authority
type SignedMessage struct {
	Hash        common.Hash
	Number      uint64
	Signature   [64]byte // ed25519.SignatureLength
	AuthorityID ed25519.PublicKeyBytes
}

// String returns the SignedMessage as a string
func (m *SignedMessage) String() string {
	return fmt.Sprintf("hash=%s number=%d authorityID=0x%x", m.Hash, m.Number, m.AuthorityID)
}

// VoteMessage represents a network-level vote message
// https://github.com/paritytech/substrate/blob/master/client/finality-grandpa/src/communication/gossip.rs#L336
type VoteMessage struct {
	Round   uint64
	SetID   uint64
	Stage   subround // 0 for pre-vote, 1 for pre-commit
	Message *SignedMessage
}

// Type returns voteType or precommitType
func (v *VoteMessage) Type() byte {
	return byte(v.Stage)
}

// ToConsensusMessage converts the VoteMessage into a network-level consensus message
func (v *VoteMessage) ToConsensusMessage() (*ConsensusMessage, error) {
	enc, err := scale.Encode(v)
	if err != nil {
		return nil, err
	}

	typ := byte(v.Stage)
	return &ConsensusMessage{
		ConsensusEngineID: types.GrandpaEngineID,
		Data:              append([]byte{typ}, enc...),
	}, nil
}

// FinalizationMessage represents a network finalization message
type FinalizationMessage struct {
	Round         uint64
	Vote          *Vote
	Justification []*Justification
}

// Type returns finalizationType
func (f *FinalizationMessage) Type() byte {
	return finalizationType
}

// ToConsensusMessage converts the FinalizationMessage into a network-level consensus message
func (f *FinalizationMessage) ToConsensusMessage() (*ConsensusMessage, error) {
	enc, err := scale.Encode(f)
	if err != nil {
		return nil, err
	}

	return &ConsensusMessage{
		ConsensusEngineID: types.GrandpaEngineID,
		Data:              append([]byte{finalizationType}, enc...),
	}, nil
}

func (s *Service) newFinalizationMessage(header *types.Header, round uint64) *FinalizationMessage {
	return &FinalizationMessage{
		Round:         round,
		Vote:          NewVoteFromHeader(header),
		Justification: s.justification[round],
	}
}

type catchUpRequest struct {
	Round uint64
	SetID uint64
}

func newCatchUpRequest(round, setID uint64) *catchUpRequest {
	return &catchUpRequest{
		Round: round,
		SetID: setID,
	}
}

// Type returns catchUpRequestType
func (r *catchUpRequest) Type() byte {
	return catchUpRequestType
}

// ToConsensusMessage converts the catchUpRequest into a network-level consensus message
func (r *catchUpRequest) ToConsensusMessage() (*ConsensusMessage, error) {
	enc, err := scale.Encode(r)
	if err != nil {
		return nil, err
	}

	return &ConsensusMessage{
		ConsensusEngineID: types.GrandpaEngineID,
		Data:              append([]byte{catchUpRequestType}, enc...),
	}, nil
}

type catchUpResponse struct {
	Round                  uint64
	SetID                  uint64
	PreVoteJustification   FullJustification
	PreCommitJustification FullJustification
	Hash                   common.Hash
	Number                 uint64
}

func (s *Service) newCatchUpResponse(round, setID uint64) (*catchUpResponse, error) {
	header, err := s.blockState.GetFinalizedHeader(round, setID)
	if err != nil {
		return nil, err
	}

	has, err := s.blockState.HasJustification(header.Hash())
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, ErrNoJustification
	}

	just, err := s.blockState.GetJustification(header.Hash())
	if err != nil {
		return nil, err
	}

	r := &bytes.Buffer{}
	_, err = r.Write(just)
	if err != nil {
		return nil, err
	}

	pvj, err := FullJustification{}.Decode(r)
	if err != nil {
		return nil, err
	}

	pcj, err := FullJustification{}.Decode(r)
	if err != nil {
		return nil, err
	}

	return &catchUpResponse{
		Round:                  round,
		SetID:                  setID,
		PreVoteJustification:   pvj,
		PreCommitJustification: pcj,
		Hash:                   header.Hash(),
		Number:                 header.Number.Uint64(),
	}, nil
}

// Type returns catchUpResponseType
func (r *catchUpResponse) Type() byte {
	return catchUpResponseType
}

// ToConsensusMessage converts the catchUpResponse into a network-level consensus message
func (r *catchUpResponse) ToConsensusMessage() (*ConsensusMessage, error) {
	enc, err := scale.Encode(r)
	if err != nil {
		return nil, err
	}

	return &ConsensusMessage{
		ConsensusEngineID: types.GrandpaEngineID,
		Data:              append([]byte{catchUpResponseType}, enc...),
	}, nil
}
