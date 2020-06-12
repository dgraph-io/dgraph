package grandpa

import (
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
		// TODO: scale should be able to handle nil pointers
		mi, err = scale.Decode(msg.Data[1:], &FinalizationMessage{
			Vote: new(Vote),
			//Justification: []*Justification{},
		})
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

// VoteMessage represents a network-level vote message
// https://github.com/paritytech/substrate/blob/master/client/finality-grandpa/src/communication/gossip.rs#L336
type VoteMessage struct {
	SetID   uint64
	Round   uint64
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

// Justification represents a justification for a finalized block
//nolint:structcheck
type Justification struct {
	Vote      *Vote    //nolint:unused
	Signature []byte   //nolint:unused
	Pubkey    [32]byte //nolint:unused
}

// Decode returns the SCALE decoded Justification
func (j *Justification) Decode(r io.Reader) (*Justification, error) {
	sd := &scale.Decoder{Reader: r}
	i, err := sd.Decode(j)
	return i.(*Justification), err
}

// FinalizationMessage represents a network finalization message
//nolint:structcheck
type FinalizationMessage struct {
	Round uint64
	Vote  *Vote
	//Justification []*Justification //nolint:unused
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

func (s *Service) newFinalizationMessage(header *types.Header, round uint64) (*FinalizationMessage, error) { //nolint
	return &FinalizationMessage{
		Round: round,
		Vote:  NewVoteFromHeader(header),
		// TODO: add justification
	}, nil
}
