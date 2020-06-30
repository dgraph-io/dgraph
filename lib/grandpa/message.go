package grandpa

import (
	"fmt"
	"io"

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
	// TODO: determine correct prefixes
	voteType         byte = 0
	finalizationType byte = 1
)

// DecodeMessage decodes a network-level consensus message into a GRANDPA VoteMessage or FinalizationMessage
func (s *Service) DecodeMessage(msg *ConsensusMessage) (m FinalityMessage, err error) {
	var mi interface{}

	switch msg.Data[0] {
	case voteType:
		mi, err = scale.Decode(msg.Data[1:], &VoteMessage{Message: new(SignedMessage)})
		m = mi.(*VoteMessage)
	case finalizationType:
		mi, err = scale.Decode(msg.Data[1:], &FinalizationMessage{})
		m = mi.(*FinalizationMessage)
	default:
		return nil, ErrInvalidMessageType
	}

	if err != nil {
		return nil, err
	}

	return m, nil
}

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

// ToConsensusMessage converts the VoteMessage into a network-level consensus message
func (v *VoteMessage) ToConsensusMessage() (*ConsensusMessage, error) {
	enc, err := scale.Encode(v)
	if err != nil {
		return nil, err
	}

	return &ConsensusMessage{
		ConsensusEngineID: types.GrandpaEngineID,
		Data:              append([]byte{voteType}, enc...),
	}, nil
}

// GetFinalizedHash returns ErrNotFinalizationMessage
func (v *VoteMessage) GetFinalizedHash() (common.Hash, error) {
	return common.Hash{}, ErrNotFinalizationMessage
}

// GetRound returns the round associated with the VoteMessage
func (v *VoteMessage) GetRound() uint64 {
	return v.Round
}

// Justification represents a justification for a finalized block
type Justification struct {
	Vote        *Vote
	Signature   [64]byte
	AuthorityID ed25519.PublicKeyBytes
}

// Encode returns the SCALE encoded Justification
func (j *Justification) Encode() ([]byte, error) {
	enc, err := j.Vote.Encode()
	if err != nil {
		return nil, err
	}

	enc = append(enc, j.Signature[:]...)
	enc = append(enc, j.AuthorityID[:]...)
	return enc, nil
}

// Decode returns the SCALE decoded Justification
func (j *Justification) Decode(r io.Reader) (*Justification, error) {
	sd := &scale.Decoder{Reader: r}
	i, err := sd.Decode(j)
	return i.(*Justification), err
}

// FinalizationMessage represents a network finalization message
type FinalizationMessage struct {
	Round         uint64
	Vote          *Vote
	Justification []*Justification
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

// GetFinalizedHash returns the hash of the block that's being finalized
func (f *FinalizationMessage) GetFinalizedHash() (common.Hash, error) {
	return f.Vote.hash, nil
}

// GetRound returns the round associated with the FinalizationMessage
func (f *FinalizationMessage) GetRound() uint64 {
	return f.Round
}

func (s *Service) newFinalizationMessage(header *types.Header, round uint64) *FinalizationMessage {
	return &FinalizationMessage{
		Round:         round,
		Vote:          NewVoteFromHeader(header),
		Justification: s.justification[round],
	}
}
